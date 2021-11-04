package use

import (
	"context"
	"fmt"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/loft-sh/loftctl/pkg/vcluster"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	"io"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	Space                        string
	Cluster                      string
	Print                        bool
	PrintToken                   bool
	DisableDirectClusterEndpoint bool

	Out io.Writer
	Log log.Logger
}

// NewVirtualClusterCmd creates a new command
func NewVirtualClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VirtualClusterCmd{
		GlobalFlags: globalFlags,
		Out:         os.Stdout,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
################## loft use vcluster ##################
#######################################################
Creates a new kube context for the given virtual cluster.

Example:
loft use vcluster 
loft use vcluster myvcluster
loft use vcluster myvcluster --cluster mycluster
loft use vcluster myvcluster --cluster mycluster --space myspace 
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace use vcluster ################
#######################################################
Creates a new kube context for the given virtual cluster.

Example:
devspace use vcluster 
devspace use vcluster myvcluster
devspace use vcluster myvcluster --cluster mycluster
devspace use vcluster myvcluster --cluster mycluster --space myspace 
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Creates a kube context for the given virtual cluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			if cmd.Print == false && cmd.PrintToken == false {
				upgrade.PrintNewerVersionWarning()
			}

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.Space, "space", "", "The space to use")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().BoolVar(&cmd.Print, "print", false, "When enabled prints the context to stdout")
	c.Flags().BoolVar(&cmd.DisableDirectClusterEndpoint, "disable-direct-cluster-endpoint", false, "When enabled does not use an available direct cluster endpoint to connect to the vcluster")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	err = client.VerifyVersion(baseClient)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	virtualClusterName := ""
	if len(args) > 0 {
		virtualClusterName = args[0]
	}

	virtualClusterName, spaceName, clusterName, err := helper.SelectVirtualClusterAndSpaceAndClusterName(baseClient, virtualClusterName, cmd.Space, cmd.Cluster, cmd.Log)
	if err != nil {
		return err
	}

	// check if the cluster exists
	cluster, err := managementClient.Loft().ManagementV1().Clusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			return fmt.Errorf("cluster '%s' does not exist, or you don't have permission to use it", clusterName)
		}

		return err
	}

	// get token for virtual cluster
	if cmd.Print == false && cmd.PrintToken == false {
		cmd.Log.StartWait("Waiting for virtual cluster to become ready...")
	}
	err = vcluster.WaitForVCluster(context.TODO(), baseClient, clusterName, spaceName, virtualClusterName, cmd.Log)
	cmd.Log.StopWait()
	if err != nil {
		return err
	}

	// create kube context options
	contextOptions, err := CreateVClusterContextOptions(baseClient, cmd.Config, cluster, spaceName, virtualClusterName, cmd.DisableDirectClusterEndpoint, true, cmd.Log)
	if err != nil {
		return err
	}

	// check if we should print or update the config
	if cmd.Print {
		err = kubeconfig.PrintKubeConfigTo(contextOptions, cmd.Out)
		if err != nil {
			return err
		}
	} else {
		// update kube config
		err = kubeconfig.UpdateKubeConfig(contextOptions)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
	}

	return nil
}

func CreateVClusterContextOptions(baseClient client.Client, config string, cluster *managementv1.Cluster, spaceName, virtualClusterName string, disableClusterGateway, setActive bool, log log.Logger) (kubeconfig.ContextOptions, error) {
	contextOptions := kubeconfig.ContextOptions{
		Name:       kubeconfig.VirtualClusterContextName(cluster.Name, spaceName, virtualClusterName),
		ConfigPath: config,
		SetActive:  setActive,
	}
	if disableClusterGateway == false && cluster.Annotations != nil && cluster.Annotations[LoftDirectClusterEndpoint] != "" {
		contextOptions = ApplyDirectClusterEndpointOptions(contextOptions, cluster, "/kubernetes/virtualcluster/"+spaceName+"/"+virtualClusterName, log)
		_, err := baseClient.DirectClusterEndpointToken(true)
		if err != nil {
			log.Errorf("Retrieving direct cluster endpoint token: %v", err)
		}
	} else {
		contextOptions.Server = baseClient.Config().Host + "/kubernetes/virtualcluster/" + cluster.Name + "/" + spaceName + "/" + virtualClusterName
		contextOptions.InsecureSkipTLSVerify = baseClient.Config().Insecure
	}

	data, err := retrieveCaData(cluster)
	if err != nil {
		return kubeconfig.ContextOptions{}, err
	}
	contextOptions.CaData = data
	return contextOptions, nil
}
