package client

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var cacheFolder = ".loft"

// DefaultCacheConfig is the path to the config
var DefaultCacheConfig = "config.json"

const (
	LoginPath     = "%s/login?cli=true"
	RedirectPath  = "%s/spaces"
	AccessKeyPath = "%s/profile/access-keys"
)

func init() {
	hd, _ := homedir.Dir()
	cacheFolder = filepath.Join(hd, cacheFolder)
	DefaultCacheConfig = filepath.Join(cacheFolder, DefaultCacheConfig)
}

type Client interface {
	Management() (kube.Interface, error)
	ManagementConfig() (*rest.Config, error)

	Cluster(cluster string) (kube.Interface, error)
	ClusterConfig(cluster string) (*rest.Config, error)
	
	VirtualCluster(cluster, namespace, virtualCluster string) (kube.Interface, error)
	VirtualClusterConfig(cluster, namespace, virtualCluster string) (*rest.Config, error)

	Login(host string, insecure bool, log log.Logger) error
	LoginWithAccessKey(host, accessKey string, insecure bool) error

	Config() *Config
	Save() error
}

func NewClientFromPath(path string) (Client, error) {
	c := &client{
		configPath: path,
	}

	err := c.initConfig()
	if err != nil {
		return nil, err
	}

	return c, nil
}

type client struct {
	configOnce sync.Once
	configPath string
	config     *Config
}

func (c *client) initConfig() error {
	var retErr error
	c.configOnce.Do(func() {
		err := os.MkdirAll(filepath.Dir(c.configPath), 0755)
		if err != nil {
			retErr = err
			return
		}

		// load the config or create new one if not found
		content, err := ioutil.ReadFile(c.configPath)
		if err != nil {
			if os.IsNotExist(err) {
				c.config = NewConfig()
				return
			}

			retErr = err
			return
		}

		config := &Config{}
		err = json.Unmarshal(content, config)
		if err != nil {
			retErr = err
			return
		}

		c.config = config
	})

	return retErr
}

func (c *client) Save() error {
	if c.config == nil {
		return errors.New("no config to write")
	}
	if c.config.TypeMeta.Kind == "" {
		c.config.TypeMeta.Kind = "Config"
	}
	if c.config.TypeMeta.APIVersion == "" {
		c.config.TypeMeta.APIVersion = "storage.loft.sh/v1"
	}

	out, err := json.Marshal(c.config)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(c.configPath, out, 0666)
}

func (c *client) ManagementConfig() (*rest.Config, error) {
	return c.restConfig("/kubernetes/management")
}

func (c *client) Management() (kube.Interface, error) {
	restConfig, err := c.ManagementConfig()
	if err != nil {
		return nil, err
	}

	return kube.NewForConfig(restConfig)
}

func (c *client) ClusterConfig(cluster string) (*rest.Config, error) {
	return c.restConfig("/kubernetes/cluster/" + cluster)
}

func (c *client) Cluster(cluster string) (kube.Interface, error) {
	restConfig, err := c.ClusterConfig(cluster)
	if err != nil {
		return nil, err
	}

	return kube.NewForConfig(restConfig)
}

func (c *client) VirtualClusterConfig(cluster, namespace, virtualCluster string) (*rest.Config, error) {
	return c.restConfig("/kubernetes/virtualcluster/" + cluster + "/" + namespace + "/" + virtualCluster)
}

func (c *client) VirtualCluster(cluster, namespace, virtualCluster string) (kube.Interface, error) {
	restConfig, err := c.VirtualClusterConfig(cluster, namespace, virtualCluster)
	if err != nil {
		return nil, err
	}

	return kube.NewForConfig(restConfig)
}

func (c *client) Config() *Config {
	return c.config
}

type keyStruct struct {
	Key string
}

func verifyHost(host string) error {
	if strings.HasPrefix(host, "https") == false {
		return fmt.Errorf("cannot log into a non https loft instance '%s', please make sure you have TLS enabled", host)
	}

	return nil
}

func (c *client) Login(host string, insecure bool, log log.Logger) error {
	var (
		loginUrl   = fmt.Sprintf(LoginPath, host)
		key        keyStruct
		keyChannel = make(chan keyStruct)
	)

	err := verifyHost(host)
	if err != nil {
		return err
	}

	server := startServer(fmt.Sprintf(RedirectPath, host), keyChannel, log)
	err = open.Run(fmt.Sprintf(LoginPath, host))
	if err != nil {
		return fmt.Errorf("couldn't open the login page in a browser: %v. Please use the --access-key flag for the login command. You can generate an access key here: %s", err, fmt.Sprintf(AccessKeyPath, host))
	} else {
		log.Infof("If the browser does not open automatically, please navigate to %s", loginUrl)
		log.StartWait("Logging into loft...")
		defer log.StopWait()

		key = <-keyChannel
	}

	err = server.Shutdown(context.Background())
	if err != nil {
		return err
	}

	close(keyChannel)
	return c.LoginWithAccessKey(host, key.Key, insecure)
}

func (c *client) LoginWithAccessKey(host, accessKey string, insecure bool) error {
	err := verifyHost(host)
	if err != nil {
		return err
	}
	if c.config.Host == host && c.config.AccessKey == accessKey {
		return nil
	}

	// delete old access key if were logged in before
	if c.config.AccessKey != "" {
		managementClient, err := c.Management()
		if err == nil {
			self, err := managementClient.Loft().ManagementV1().Selves().Create(context.TODO(), &managementv1.Self{}, metav1.CreateOptions{})
			if err == nil && self.Status.AccessKey != "" && self.Status.AccessKeyType == storagev1.AccessKeyTypeLogin {
				_ = managementClient.Loft().ManagementV1().OwnedAccessKeys().Delete(context.TODO(), self.Status.AccessKey, metav1.DeleteOptions{})
			}
		}
	}

	c.config.Host = host
	c.config.Insecure = insecure
	c.config.AccessKey = accessKey
	
	// verify the connection works
	managementClient, err := c.Management()
	if err != nil {
		return errors.Wrap(err, "create management client")
	}
	
	// try to get self
	_, err = managementClient.Loft().ManagementV1().Selves().Create(context.TODO(), &managementv1.Self{}, metav1.CreateOptions{})
	if err != nil {
		if urlError, ok := err.(*url.Error); ok {
			if _, ok := urlError.Err.(x509.UnknownAuthorityError); ok {
				return fmt.Errorf("unsafe login endpoint '%s', if you wish to login into an insecure loft endpoint run with the '--insecure' flag", c.config.Host)
			}
		}
		
		return errors.Errorf("error logging in: %v", err)
	}
	
	return c.Save()
}

func (c *client) restConfig(hostSuffix string) (*rest.Config, error) {
	if c.config == nil {
		return nil, errors.New("no config loaded")
	} else if c.config.Host == "" || c.config.AccessKey == "" {
		return nil, errors.New("not logged in, please make sure you have run 'loft login [loft-url]'")
	}

	// build a rest config
	config, err := getRestConfig(c.config.Host+hostSuffix, c.config.AccessKey, c.config.Insecure)
	if err != nil {
		return nil, err
	}

	return config, err
}

func getRestConfig(host, token string, insecure bool) (*rest.Config, error) {
	contextName := "local"
	kubeConfig := clientcmdapi.NewConfig()
	kubeConfig.Contexts = map[string]*clientcmdapi.Context{
		contextName: &clientcmdapi.Context{
			Cluster:  contextName,
			AuthInfo: contextName,
		},
	}
	kubeConfig.Clusters = map[string]*clientcmdapi.Cluster{
		contextName: &clientcmdapi.Cluster{
			Server:                host,
			InsecureSkipTLSVerify: insecure,
		},
	}
	kubeConfig.AuthInfos = map[string]*clientcmdapi.AuthInfo{
		contextName: &clientcmdapi.AuthInfo{
			Token: token,
		},
	}
	kubeConfig.CurrentContext = contextName
	config, err := clientcmd.NewDefaultClientConfig(*kubeConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}

	return config, nil
}

func startServer(redirectURI string, keyChannel chan keyStruct, log log.Logger) *http.Server {
	srv := &http.Server{Addr: ":25843"}

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		keys, ok := r.URL.Query()["key"]
		if !ok || len(keys[0]) == 0 {
			log.Warn("Login: the key used to login is not valid")
			return
		}

		keyChannel <- keyStruct{
			Key: keys[0],
		}
		http.Redirect(w, r, redirectURI, http.StatusSeeOther)
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			// cannot panic, because this probably is an intentional close
		}
	}()

	// returning reference so caller can call Shutdown()
	return srv
}
