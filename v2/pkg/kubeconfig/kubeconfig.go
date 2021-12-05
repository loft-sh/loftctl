package kubeconfig

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/pkg/apis/clientauthentication/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

type ContextOptions struct {
	Name                         string
	Server                       string
	CaData                       []byte
	ConfigPath                   string
	InsecureSkipTLSVerify        bool
	DirectClusterEndpointEnabled bool

	Token            string
	CurrentNamespace string
	SetActive        bool
}

func SpaceContextName(clusterName, namespaceName string) string {
	contextName := "loft_"
	if namespaceName != "" {
		contextName += namespaceName + "_"
	}

	contextName += clusterName
	return contextName
}

func VirtualClusterContextName(clusterName, namespaceName, virtualClusterName string) string {
	return "loft-vcluster_" + virtualClusterName + "_" + namespaceName + "_" + clusterName
}

func ParseContext(contextName string) (isLoftContext bool, cluster string, namespace string, vCluster string) {
	splitted := strings.Split(contextName, "_")
	if len(splitted) == 0 || (splitted[0] != "loft" && splitted[0] != "loft-vcluster") {
		return false, "", "", ""
	}

	// cluster or space context
	if splitted[0] == "loft" {
		if len(splitted) > 3 || len(splitted) == 1 {
			return false, "", "", ""
		} else if len(splitted) == 2 {
			return true, splitted[1], "", ""
		}

		return true, splitted[2], splitted[1], ""
	}

	// vCluster context
	if len(splitted) != 4 {
		return false, "", "", ""
	}

	return true, splitted[3], splitted[2], splitted[1]
}

func CurrentContext() (string, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return "", err
	}

	return config.CurrentContext, nil
}

// DeleteContext deletes the context with the given name from the kube config
func DeleteContext(contextName string) error {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return err
	}

	delete(config.Contexts, contextName)
	delete(config.Clusters, contextName)
	delete(config.AuthInfos, contextName)

	if config.CurrentContext == contextName {
		config.CurrentContext = ""
		for name := range config.Contexts {
			config.CurrentContext = name
			break
		}
	}

	// Save the config
	return clientcmd.ModifyConfig(clientcmd.NewDefaultClientConfigLoadingRules(), config, false)
}

func updateKubeConfig(contextName string, cluster *api.Cluster, authInfo *api.AuthInfo, namespaceName string, setActive bool) error {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).RawConfig()
	if err != nil {
		return err
	}

	config.Clusters[contextName] = cluster
	config.AuthInfos[contextName] = authInfo

	// Update kube context
	context := api.NewContext()
	context.Cluster = contextName
	context.AuthInfo = contextName
	context.Namespace = namespaceName

	config.Contexts[contextName] = context
	if setActive {
		config.CurrentContext = contextName
	}

	// Save the config
	return clientcmd.ModifyConfig(clientcmd.NewDefaultClientConfigLoadingRules(), config, false)
}

func printKubeConfigTo(contextName string, cluster *api.Cluster, authInfo *api.AuthInfo, namespaceName string, writer io.Writer) error {
	config := api.NewConfig()

	config.Clusters[contextName] = cluster
	config.AuthInfos[contextName] = authInfo

	// Update kube context
	context := api.NewContext()
	context.Cluster = contextName
	context.AuthInfo = contextName
	context.Namespace = namespaceName

	config.Contexts[contextName] = context
	config.CurrentContext = contextName

	// set kind & version
	config.APIVersion = "v1"
	config.Kind = "Config"

	out, err := clientcmd.Write(*config)
	if err != nil {
		return err
	}

	_, err = writer.Write(out)
	return err
}

// UpdateKubeConfig updates the kube config and adds the virtual cluster context
func UpdateKubeConfig(options ContextOptions) error {
	contextName, cluster, authInfo, err := createContext(options)
	if err != nil {
		return err
	}

	// we don't want to set the space name here as the default namespace in the virtual cluster, because it couldn't exist
	return updateKubeConfig(contextName, cluster, authInfo, options.CurrentNamespace, options.SetActive)
}

// PrintKubeConfigTo prints the given config to the writer
func PrintKubeConfigTo(options ContextOptions, writer io.Writer) error {
	contextName, cluster, authInfo, err := createContext(options)
	if err != nil {
		return err
	}

	// we don't want to set the space name here as the default namespace in the virtual cluster, because it couldn't exist
	return printKubeConfigTo(contextName, cluster, authInfo, options.CurrentNamespace, writer)
}

// PrintTokenKubeConfig writes the kube config to the os.Stdout
func PrintTokenKubeConfig(restConfig *rest.Config, token string) error {
	contextName := "default"
	cluster := api.NewCluster()
	cluster.Server = restConfig.Host
	cluster.InsecureSkipTLSVerify = restConfig.Insecure
	cluster.CertificateAuthority = restConfig.CAFile
	cluster.CertificateAuthorityData = restConfig.CAData
	cluster.TLSServerName = restConfig.ServerName

	authInfo := api.NewAuthInfo()
	authInfo.Token = token

	return printKubeConfigTo(contextName, cluster, authInfo, "", os.Stdout)
}

func createContext(options ContextOptions) (string, *api.Cluster, *api.AuthInfo, error) {
	contextName := options.Name
	cluster := api.NewCluster()
	cluster.Server = options.Server
	cluster.CertificateAuthorityData = options.CaData
	cluster.InsecureSkipTLSVerify = options.InsecureSkipTLSVerify

	authInfo := api.NewAuthInfo()
	if options.Token != "" {
		authInfo.Token = options.Token
	} else {
		command, err := os.Executable()
		if err != nil {
			return "", nil, nil, err
		}

		absConfigPath, err := filepath.Abs(options.ConfigPath)
		if err != nil {
			return "", nil, nil, err
		}

		authInfo.Exec = &api.ExecConfig{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
			Command:    command,
			Args:       []string{"token", "--silent", "--config", absConfigPath},
		}
		if options.DirectClusterEndpointEnabled {
			authInfo.Exec.Args = append(authInfo.Exec.Args, "--direct-cluster-endpoint")
		}
	}

	return contextName, cluster, authInfo, nil
}
