package delete

import (
	"context"
	"time"

	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	Space         string
	Cluster       string
	DeleteContext bool
	DeleteSpace   bool
	Wait          bool

	Log log.Logger
}

// NewVirtualClusterCmd creates a new command
func NewVirtualClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VirtualClusterCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
############ loft delete virtualcluster ###############
#######################################################
Deletes a virtual cluster from a cluster

Example:
loft delete vcluster myvirtualcluster 
loft delete vcluster myvirtualcluster --cluster mycluster
loft delete vcluster myvirtualcluster --space myspace --cluster mycluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
########## devspace delete virtualcluster #############
#######################################################
Deletes a virtual cluster from a cluster

Example:
devspace delete vcluster myvirtualcluster 
devspace delete vcluster myvirtualcluster --cluster mycluster
devspace delete vcluster myvirtualcluster --space myspace --cluster mycluster
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster [name]",
		Short: "Deletes a virtual cluster from a cluster",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.Space, "space", "", "The space to use")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().BoolVar(&cmd.DeleteContext, "delete-context", true, "If the corresponding kube context should be deleted if there is any")
	c.Flags().BoolVar(&cmd.DeleteSpace, "delete-space", false, "Should the corresponding space be deleted")
	c.Flags().BoolVar(&cmd.Wait, "wait", false, "Termination of this command waits for space to be deleted. Without the flag delete-space, this flag has no effect.")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(args []string) error {
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

	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	gracePeriod := int64(0)
	err = clusterClient.Agent().StorageV1().VirtualClusters(spaceName).Delete(context.TODO(), virtualClusterName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
	if err != nil {
		return errors.Wrap(err, "delete virtual cluster")
	}

	cmd.Log.Donef("Successfully deleted virtual cluster %s in space %s in cluster %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// update kube config
	if cmd.DeleteContext {
		err = kubeconfig.DeleteContext(kubeconfig.VirtualClusterContextName(clusterName, spaceName, virtualClusterName))
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully deleted kube context for virtual cluster %s", ansi.Color(virtualClusterName, "white+b"))
	}

	// delete space
	if cmd.DeleteSpace {
		err = clusterClient.Agent().ClusterV1().Spaces().Delete(context.TODO(), spaceName, metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		// wait for termination
		if cmd.Wait {
			cmd.Log.StartWait("Waiting for space to be deleted")
			for isSpaceStillThere(clusterClient, spaceName) {
				time.Sleep(time.Second)
			}
			cmd.Log.StopWait()
		}

		cmd.Log.Donef("Successfully deleted space %s", spaceName)
	}

	return nil
}
