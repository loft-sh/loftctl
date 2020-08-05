package use

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
)

// ClusterCmd holds the cmd flags
type ClusterCmd struct {
	*flags.GlobalFlags

	Print bool
	log   log.Logger
}

// NewClusterCmd creates a new command
func NewClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ClusterCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	description := `
#######################################################
################## loft use cluster ###################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
loft use cluster mycluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace use cluster #################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
devspace use cluster mycluster
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "cluster",
		Short: "Creates a kube context for the given cluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().BoolVar(&cmd.Print, "print", false, "When enabled prints the context to stdout")
	return c
}

// Run executes the command
func (cmd *ClusterCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	client, err := baseClient.Management()
	if err != nil {
		return err
	}

	// determine cluster name
	clusterName := ""
	if len(args) == 0 {
		clusterName, err = helper.SelectCluster(baseClient, cmd.log)
		if err != nil {
			return err
		}
	} else {
		clusterName = args[0]
	}

	// check if we cluster exists
	_, err = client.Loft().ManagementV1().Clusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			return fmt.Errorf("cluster '%s' does not exist, or you don't have permission to use it", clusterName)
		}

		return err
	}

	// check if we should print or update the config
	if cmd.Print {
		err = kubeconfig.PrintKubeConfigTo(baseClient.Config(), clusterName, "", os.Stdout)
		if err != nil {
			return err
		}
	} else {
		// update kube config
		err = kubeconfig.UpdateKubeConfig(baseClient.Config(), clusterName, "", true)
		if err != nil {
			return err
		}

		cmd.log.Donef("Successfully updated kube context to use cluster %s", ansi.Color(clusterName, "white+b"))
	}

	return nil
}
