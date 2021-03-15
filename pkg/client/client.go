package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	types "github.com/loft-sh/api/pkg/auth"
	"github.com/loft-sh/api/pkg/token"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"gopkg.in/square/go-jose.v2/jwt"
	"io"
	"io/ioutil"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var cacheFolder = ".loft"

// DefaultCacheConfig is the path to the config
var DefaultCacheConfig = "config.json"

const (
	LoginPath            = "%s/login?cli=true"
	RedirectPath         = "%s/clusters"
	TokenPath            = "%s/auth/token"
	OIDCTokenPath        = "%s/auth/oidc/token"
	OIDCRefreshTokenPath = "%s/auth/oidc/refresh"
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

	Login(host string, insecure bool, log log.Logger) error
	LoginWithAccessKey(host, username, accessKey string, insecure bool) error

	AuthInfo() (*token.Loft, error)
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

func (c *client) Config() *Config {
	return c.config
}

type usernameKey struct {
	Username  string
	Key       string
	OIDCToken string
}

func verifyHost(host string) error {
	if strings.HasPrefix(host, "https") == false {
		return fmt.Errorf("cannot log into a non https loft instance '%s', please make sure you have TLS enabled", host)
	}

	return nil
}

func (c *client) Login(host string, insecure bool, log log.Logger) error {
	var (
		loginUrl           = fmt.Sprintf(LoginPath, host)
		userKey            usernameKey
		usernameKeyChannel = make(chan usernameKey)
	)

	err := verifyHost(host)
	if err != nil {
		return err
	}

	server := startServer(fmt.Sprintf(RedirectPath, host), usernameKeyChannel, log)
	err = open.Run(fmt.Sprintf(LoginPath, host))
	if err != nil {
		return fmt.Errorf("couldn't open a browser window: %v. Please login via an access token", err)
	} else {
		log.Infof("If the browser does not open automatically, please navigate to %s", loginUrl)
		log.StartWait("Logging into loft...")
		defer log.StopWait()

		userKey = <-usernameKeyChannel
	}

	err = server.Shutdown(context.Background())
	if err != nil {
		return err
	}

	close(usernameKeyChannel)
	if userKey.OIDCToken != "" {
		out, err := base64.StdEncoding.DecodeString(userKey.OIDCToken)
		if err != nil {
			return err
		}

		oidcToken := &types.OIDCToken{}
		err = json.Unmarshal(out, oidcToken)
		if err != nil {
			return err
		}

		return c.loginWithOIDCToken(host, oidcToken, insecure)
	}

	return c.LoginWithAccessKey(host, userKey.Username, userKey.Key, insecure)
}

func (c *client) loginWithOIDCToken(host string, oidcToken *types.OIDCToken, insecure bool) error {
	err := verifyHost(host)
	if err != nil {
		return err
	}

	if c.config.Host == host && c.config.OIDCToken == oidcToken.IDToken && c.config.OIDCRefreshToken == oidcToken.RefreshToken {
		return nil
	}

	c.config.Host = host
	c.config.Insecure = insecure
	c.config.Username = ""
	c.config.AccessKey = ""
	c.config.OIDCToken = oidcToken.IDToken
	c.config.OIDCAccessToken = oidcToken.AccessToken
	c.config.OIDCRefreshToken = oidcToken.RefreshToken
	c.config.Token = ""
	c.config.TokenExp = 0
	return c.refreshToken()
}

func (c *client) LoginWithAccessKey(host, username, accessKey string, insecure bool) error {
	err := verifyHost(host)
	if err != nil {
		return err
	}

	if c.config.Host == host && c.config.Username == username && c.config.AccessKey == accessKey {
		return nil
	}

	c.config.Host = host
	c.config.Insecure = insecure
	c.config.Username = username
	c.config.AccessKey = accessKey
	c.config.OIDCToken = ""
	c.config.OIDCAccessToken = ""
	c.config.OIDCRefreshToken = ""
	c.config.Token = ""
	c.config.TokenExp = 0
	return c.refreshToken()
}

func (c *client) AuthInfo() (*token.Loft, error) {
	err := c.refreshToken()
	if err != nil {
		return nil, err
	}

	// parse token
	parsedTok, err := jwt.ParseSigned(c.config.Token)
	if err != nil {
		return nil, err
	}

	// extract the claims
	public := &jwt.Claims{}
	private := &token.PrivateClaims{}
	err = parsedTok.UnsafeClaimsWithoutVerification(public, private)
	if err != nil {
		return nil, err
	}

	return &private.Loft, nil
}

func (c *client) restConfig(hostSuffix string) (*rest.Config, error) {
	if c.config == nil {
		return nil, errors.New("no config loaded")
	}

	// refresh the token
	err := c.refreshToken()
	if err != nil {
		return nil, err
	}

	// build a rest config
	config, err := getRestConfig(c.config.Host+hostSuffix, c.config.Token, c.config.Insecure)
	if err != nil {
		return nil, err
	}

	return config, err
}

func (c *client) refreshToken() error {
	if c.config == nil {
		return errors.New("no config loaded")
	} else if c.config.Host == "" || ((c.config.Username == "" || c.config.AccessKey == "") && c.config.OIDCToken == "") {
		return errors.New("not logged in, please make sure you have run 'loft login [loft-url]'")
	}

	now := time.Now().Unix()
	refresh := c.config.Token == "" || c.config.TokenExp-600 <= now
	if refresh == false {
		return nil
	}

	var client *http.Client
	if c.config.Insecure {
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	} else {
		client = &http.Client{}
	}

	var (
		tokenBytes []byte
		err        error
	)

	if c.config.Username != "" {
		tokenBytes, err = c.getTokenByAccessKey(client)
	} else if c.config.OIDCToken != "" {
		tokenBytes, err = c.getTokenByOIDC(client)
	}
	if err != nil {
		return err
	}

	tok := &types.Token{}
	err = json.Unmarshal(tokenBytes, tok)
	if err != nil {
		return err
	}

	// parse token
	parsedTok, err := jwt.ParseSigned(tok.Token)
	if err != nil {
		return err
	}

	// extract the claims
	public := &jwt.Claims{}
	private := &token.PrivateClaims{}
	err = parsedTok.UnsafeClaimsWithoutVerification(public, private)
	if err != nil {
		return err
	}

	c.config.Token = tok.Token
	c.config.TokenExp = now + (public.Expiry.Time().Unix() - public.IssuedAt.Time().Unix())
	return c.Save()
}

func (c *client) getTokenByOIDC(client *http.Client) ([]byte, error) {
	reader, err := newJSONReader(&types.OIDCTokenRequest{
		Token:       c.config.OIDCToken,
		AccessToken: c.config.OIDCAccessToken,
	})
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(fmt.Sprintf(OIDCTokenPath, c.config.Host), "application/json", reader)
	if err != nil {
		if urlError, ok := err.(*url.Error); ok {
			if _, ok := urlError.Err.(x509.UnknownAuthorityError); ok {
				return nil, fmt.Errorf("unsafe login endpoint '%s', if you wish to login into an insecure loft endpoint run with the '--insecure' flag", c.config.Host)
			}
		}

		return nil, err
	}
	defer resp.Body.Close()

	// try to refresh if we get unauthorized
	if resp.StatusCode == http.StatusUnauthorized {
		if c.config.OIDCRefreshToken == "" {
			return nil, fmt.Errorf("OIDC token has expired, please relogin via: loft login [url]")
		}

		reader, err := newJSONReader(&types.OIDCRefreshRequest{
			RefreshToken: c.config.OIDCRefreshToken,
		})
		if err != nil {
			return nil, err
		}
		resp, err := client.Post(fmt.Sprintf(OIDCRefreshTokenPath, c.config.Host), "application/json", reader)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		} else if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error refreshing oidc token: %v. Try to relogin via: loft login [url]", string(body))
		}

		oidcToken := &types.OIDCToken{}
		err = json.Unmarshal(body, oidcToken)
		if err != nil {
			return nil, err
		}

		// update the config
		c.config.OIDCToken = oidcToken.IDToken
		c.config.OIDCAccessToken = oidcToken.AccessToken
		c.config.OIDCRefreshToken = oidcToken.RefreshToken
		err = c.Save()
		if err != nil {
			return nil, err
		}

		// try to refetch
		reader, err = newJSONReader(&types.OIDCTokenRequest{
			Token:       c.config.OIDCToken,
			AccessToken: c.config.OIDCAccessToken,
		})
		if err != nil {
			return nil, err
		}
		tokenResponse, err := client.Post(fmt.Sprintf(OIDCTokenPath, c.config.Host), "application/json",  reader)
		if err != nil {
			return nil, err
		}
		defer tokenResponse.Body.Close()

		tokenBody, err := ioutil.ReadAll(tokenResponse.Body)
		if err != nil {
			return nil, err
		} else if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error retrieving token by oidc: %v", string(body))
		}

		return tokenBody, nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error retrieving token by oidc: %v", string(body))
	}

	return body, nil
}

func newJSONReader(in interface{}) (io.Reader, error) {
	out, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	
	return bytes.NewReader(out), nil
}

func (c *client) getTokenByAccessKey(client *http.Client) ([]byte, error) {
	reader, err := newJSONReader(&types.TokenRequest{
		Username: c.config.Username,
		Key:      c.config.AccessKey,
	})
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(fmt.Sprintf(TokenPath, c.config.Host), "application/json", reader)
	if err != nil {
		if urlError, ok := err.(*url.Error); ok {
			if _, ok := urlError.Err.(x509.UnknownAuthorityError); ok {
				return nil, fmt.Errorf("unsafe login endpoint '%s', if you wish to login into an insecure loft endpoint run with the '--insecure' flag", c.config.Host)
			}
		}

		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error retrieving token by access key: %v", string(body))
	}

	return body, nil
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

func startServer(redirectURI string, keyChannel chan usernameKey, log log.Logger) *http.Server {
	srv := &http.Server{Addr: ":25843"}

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		oidcToken, ok := r.URL.Query()["oidc_token"]
		if ok && len(oidcToken) > 0 {
			keyChannel <- usernameKey{
				OIDCToken: oidcToken[0],
			}
			http.Redirect(w, r, redirectURI, http.StatusSeeOther)
			return
		}

		username, ok := r.URL.Query()["username"]
		if !ok || len(username[0]) == 0 {
			log.Warn("Login: the username used to login is not valid")
			return
		}

		keys, ok := r.URL.Query()["key"]
		if !ok || len(keys[0]) == 0 {
			log.Warn("Login: the key used to login is not valid")
			return
		}

		keyChannel <- usernameKey{
			Username: username[0],
			Key:      keys[0],
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
