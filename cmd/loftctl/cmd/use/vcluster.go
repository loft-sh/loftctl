package use

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/loft-sh/loftctl/pkg/virtualcluster"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	"io"
	"os"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	Space      string
	Cluster    string
	Print      bool
	PrintToken bool

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

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Space, "space", "", "The space to use")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().BoolVar(&cmd.Print, "print", false, "When enabled prints the context to stdout")
	c.Flags().BoolVar(&cmd.PrintToken, "print-token", false, "When enabled prints the virtual cluster token")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
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

	// create a cluster client
	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	// get token for virtual cluster
	if cmd.Print == false && cmd.PrintToken == false {
		cmd.Log.StartWait("Waiting for virtual cluster to become ready...")
	}
	token, err := virtualcluster.GetVirtualClusterToken(context.TODO(), clusterClient, virtualClusterName, spaceName)
	cmd.Log.StopWait()
	if err != nil {
		return err
	}

	// check if we should print or update the config
	if cmd.Print {
		err = kubeconfig.PrintVirtualClusterKubeConfigTo(baseClient.Config(), clusterName, spaceName, virtualClusterName, token, cmd.Out)
		if err != nil {
			return err
		}
	} else if cmd.PrintToken {
		_, err := cmd.Out.Write([]byte(token))
		if err != nil {
			return err
		}
	} else {
		// update kube config
		err = kubeconfig.UpdateKubeConfigVirtualCluster(baseClient.Config(), clusterName, spaceName, virtualClusterName, token, true)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
	}

	return nil
}
