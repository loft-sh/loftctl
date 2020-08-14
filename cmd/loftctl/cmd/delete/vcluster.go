package delete

import (
	"context"
	"time"

	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
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
		Use:   "vcluster",
		Short: "Deletes a virtual cluster from a cluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
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
func (cmd *VirtualClusterCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	virtualClusterName := ""
	if len(args) > 0 {
		virtualClusterName = args[0]
	}

	virtualClusterName, spaceName, clusterName, err := helper.SelectVirtualClusterAndSpaceAndClusterName(baseClient, virtualClusterName, cmd.Space, cmd.Cluster, cmd.log)
	if err != nil {
		return err
	}

	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	gracePeriod := int64(0)
	err = clusterClient.Loft().StorageV1().VirtualClusters(spaceName).Delete(context.TODO(), virtualClusterName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
	if err != nil {
		return errors.Wrap(err, "delete virtual cluster")
	}

	cmd.log.Donef("Successfully deleted virtual cluster %s in space %s in cluster %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// update kube config
	if cmd.DeleteContext {
		err = kubeconfig.DeleteContext(kubeconfig.VirtualClusterContextName(clusterName, spaceName, virtualClusterName))
		if err != nil {
			return err
		}

		cmd.log.Donef("Successfully deleted kube context for virtual cluster %s", ansi.Color(virtualClusterName, "white+b"))
	}

	// delete space
	if cmd.DeleteSpace {
		err = clusterClient.CoreV1().Namespaces().Delete(context.TODO(), spaceName, metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		// wait for termination
		if cmd.Wait {
			cmd.log.StartWait("Waiting for space to be deleted")
			for isSpaceStillThere(clusterClient, spaceName) {
				time.Sleep(time.Second)
			}
			cmd.log.StopWait()
		}

		cmd.log.Donef("Successfully deleted space %s", spaceName)
	}

	return nil
}
