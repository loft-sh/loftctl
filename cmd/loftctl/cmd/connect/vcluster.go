package connect

import (
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io/ioutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	KubeConfig string
	Namespace  string
	Print      bool
	LocalPort  int

	log log.Logger
}

// NewVirtualClusterCmd creates a new command
func NewVirtualClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VirtualClusterCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
############### loft connect vcluster #################
#######################################################
This command connects to a virtual cluster directly via
port-forwarding and writes a kube config to the specified
location.

Example:
loft connect vcluster test
loft connect vcluster test --namespace test
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############# devspace connect vcluster ###############
#######################################################
This command connects to a virtual cluster directly via
port-forwarding and writes a kube config to the specified
location.

Example:
devspace connect vcluster test
devspace connect vcluster test --namespace test
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Connects to a virtual cluster in the given parent cluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.KubeConfig, "out-kube-config", "./kubeconfig.yaml", "The path to write the resulting kube config to")
	c.Flags().StringVarP(&cmd.Namespace, "namespace", "n", "", "The namespace to use")
	c.Flags().BoolVar(&cmd.Print, "print", false, "When enabled prints the context to stdout")
	c.Flags().IntVar(&cmd.LocalPort, "local-port", 8443, "The local port to forward the virtual cluster to")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(args []string) error {
	kubeConfigLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	config, err := kubeConfigLoader.ClientConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// set the namespace correctly
	if cmd.Namespace == "" {
		cmd.Namespace, _, err = kubeConfigLoader.Namespace()
		if err != nil {
			return err
		}
	}

	podName := ""
	if len(args) > 0 {
		podName = args[0] + "-0"
	}

	if podName == "" {
		podName, err = helper.SelectPod(kubeClient, cmd.Namespace, cmd.log)
		if err != nil {
			return err
		}

		cmd.log.Infof("Found vcluster pod %s in namespace %s", podName, cmd.Namespace)
	}

	// get the kube config from the container
	out, err := exec.Command("kubectl", "exec", "--namespace", cmd.Namespace, "-c", "syncer", podName, "--", "cat", "/root/.kube/config").CombinedOutput()
	if err != nil {
		return errors.New(string(out))
	}

	kubeConfig, err := clientcmd.Load(out)
	if err != nil {
		return errors.Wrap(err, "parse kube config")
	}

	// find out port we should listen to locally
	if len(kubeConfig.Clusters) != 1 {
		return fmt.Errorf("unexpected kube config")
	}

	port := ""
	for k := range kubeConfig.Clusters {
		splitted := strings.Split(kubeConfig.Clusters[k].Server, ":")
		if len(splitted) != 3 {
			return fmt.Errorf("unexpected server in kubeconfig: %s", kubeConfig.Clusters[k].Server)
		}

		port = splitted[2]
		splitted[2] = strconv.Itoa(cmd.LocalPort)
		kubeConfig.Clusters[k].Server = strings.Join(splitted, ":")
	}

	out, err = clientcmd.Write(*kubeConfig)
	if err != nil {
		return err
	}

	// write kube config to file
	if cmd.Print {
		_, err = os.Stdout.Write(out)
		if err != nil {
			return err
		}
	} else {
		err = ioutil.WriteFile(cmd.KubeConfig, out, 0666)
		if err != nil {
			return errors.Wrap(err, "write kube config")
		}

		cmd.log.Donef("Virtual cluster kube config written to: %s. You can access the cluster via `kubectl --kubeconfig %s get namespaces`", cmd.KubeConfig, cmd.KubeConfig)
	}

	forwardPorts := strconv.Itoa(cmd.LocalPort) + ":" + port
	cmd.log.Infof("Starting port forwarding on port %s", forwardPorts)
	portforwardCmd := exec.Command("kubectl", "port-forward", "--namespace", cmd.Namespace, podName, forwardPorts)
	if !cmd.Print {
		portforwardCmd.Stdout = os.Stdout
	} else {
		portforwardCmd.Stdout = ioutil.Discard
	}

	portforwardCmd.Stderr = os.Stderr
	return portforwardCmd.Run()
}
