package kubeconfig

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/loft-sh/loftctl/pkg/client"
	"k8s.io/client-go/pkg/apis/clientauthentication/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

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

// PrintKubeConfigTo prints the given config to the writer
func PrintKubeConfigTo(clientConfig *client.Config, configPath, clusterName, namespaceName string, writer io.Writer) error {
	contextName, cluster, authInfo, err := createSpaceContextInfo(clientConfig, configPath, clusterName, namespaceName)
	if err != nil {
		return err
	}

	return printKubeConfigTo(contextName, cluster, authInfo, namespaceName, writer)
}

// UpdateKubeConfig updates the kube config and adds the spaceConfig context
func UpdateKubeConfig(clientConfig *client.Config, configPath, clusterName, namespaceName string, setActive bool) error {
	contextName, cluster, authInfo, err := createSpaceContextInfo(clientConfig, configPath, clusterName, namespaceName)
	if err != nil {
		return err
	}

	// Save the config
	return updateKubeConfig(contextName, cluster, authInfo, namespaceName, setActive)
}

func createSpaceContextInfo(clientConfig *client.Config, configPath, clusterName, namespaceName string) (string, *api.Cluster, *api.AuthInfo, error) {
	contextName := SpaceContextName(clusterName, namespaceName)
	cluster := api.NewCluster()
	cluster.Server = clientConfig.Host + "/kubernetes/cluster/" + clusterName
	cluster.InsecureSkipTLSVerify = clientConfig.Insecure

	command, err := os.Executable()
	if err != nil {
		return "", nil, nil, err
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", nil, nil, err
	}

	authInfo := api.NewAuthInfo()
	authInfo.Exec = &api.ExecConfig{
		APIVersion: v1alpha1.SchemeGroupVersion.String(),
		Command:    command,
		Args:       []string{"token", "--silent", "--config", absConfigPath},
	}
	return contextName, cluster, authInfo, nil
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

func createVirtualClusterContextInfo(clientConfig *client.Config, configPath, clusterName, namespaceName, virtualClusterName string) (string, *api.Cluster, *api.AuthInfo, error) {
	contextName := VirtualClusterContextName(clusterName, namespaceName, virtualClusterName)
	cluster := api.NewCluster()
	cluster.Server = clientConfig.Host + "/kubernetes/virtualcluster/" + clusterName + "/" + namespaceName + "/" + virtualClusterName
	cluster.InsecureSkipTLSVerify = clientConfig.Insecure

	command, err := os.Executable()
	if err != nil {
		return "", nil, nil, err
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", nil, nil, err
	}

	authInfo := api.NewAuthInfo()
	authInfo.Exec = &api.ExecConfig{
		APIVersion: v1alpha1.SchemeGroupVersion.String(),
		Command:    command,
		Args:       []string{"token", "--silent", "--config", absConfigPath},
	}
	return contextName, cluster, authInfo, nil
}

// UpdateKubeConfigVirtualCluster updates the kube config and adds the virtual cluster context
func UpdateKubeConfigVirtualCluster(clientConfig *client.Config, configName, clusterName, namespaceName, virtualClusterName string, setActive bool) error {
	contextName, cluster, authInfo, err := createVirtualClusterContextInfo(clientConfig, configName, clusterName, namespaceName, virtualClusterName)
	if err != nil {
		return err
	}

	// we don't want to set the space name here as the default namespace in the virtual cluster, because it couldn't exist
	return updateKubeConfig(contextName, cluster, authInfo, "", setActive)
}

// PrintVirtualClusterKubeConfigTo prints the given config to the writer
func PrintVirtualClusterKubeConfigTo(clientConfig *client.Config, configName, clusterName, namespaceName, virtualClusterName string, writer io.Writer) error {
	contextName, cluster, authInfo, err := createVirtualClusterContextInfo(clientConfig, configName, clusterName, namespaceName, virtualClusterName)
	if err != nil {
		return err
	}

	// we don't want to set the space name here as the default namespace in the virtual cluster, because it couldn't exist
	return printKubeConfigTo(contextName, cluster, authInfo, "", writer)
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
