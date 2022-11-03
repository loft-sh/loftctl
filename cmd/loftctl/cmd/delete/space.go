package delete

import (
	"context"
	"github.com/loft-sh/loftctl/v2/pkg/client/naming"
	"time"

	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/kube"
	"github.com/loft-sh/loftctl/v2/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	Cluster       string
	Project       string
	DeleteContext bool
	Wait          bool

	Log log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
################# loft delete space ###################
#######################################################
Deletes a space from a cluster

Example:
loft delete space myspace
loft delete space myspace --project myproject
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace delete space #################
#######################################################
Deletes a space from a cluster

Example:
devspace delete space myspace
devspace delete space myspace --project myproject
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "space [name]",
		Short: "Deletes a space from a cluster",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVarP(&cmd.Project, "project", "p", "", "The project to use")
	c.Flags().BoolVar(&cmd.DeleteContext, "delete-context", true, "If the corresponding kube context should be deleted if there is any")
	c.Flags().BoolVar(&cmd.Wait, "wait", false, "Termination of this command waits for space to be deleted")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	spaceName := ""
	if len(args) > 0 {
		spaceName = args[0]
	}

	cmd.Cluster, cmd.Project, spaceName, err = helper.SelectSpaceInstanceOrSpace(baseClient, spaceName, cmd.Project, cmd.Cluster, cmd.Log)
	if err != nil {
		return err
	}

	if cmd.Project == "" {
		return cmd.legacyDeleteSpace(baseClient, spaceName)
	}

	return cmd.deleteSpace(baseClient, spaceName)
}

func (cmd *SpaceCmd) deleteSpace(baseClient client.Client, spaceName string) error {
	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	err = managementClient.Loft().ManagementV1().SpaceInstances(naming.ProjectNamespace(cmd.Project)).Delete(context.TODO(), spaceName, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrap(err, "delete space")
	}

	cmd.Log.Donef("Successfully deleted space %s in project %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Project, "white+b"))

	// update kube config
	if cmd.DeleteContext {
		err = kubeconfig.DeleteContext(kubeconfig.SpaceInstanceContextName(cmd.Project, spaceName))
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully deleted kube context for space %s", ansi.Color(spaceName, "white+b"))
	}

	// wait until deleted
	if cmd.Wait {
		cmd.Log.StartWait("Waiting for space to be deleted")
		for isSpaceInstanceStillThere(managementClient, naming.ProjectNamespace(cmd.Project), spaceName) {
			time.Sleep(time.Second)
		}
		cmd.Log.StopWait()
		cmd.Log.Done("Space is deleted")
	}

	return nil
}

func isSpaceInstanceStillThere(managementClient kube.Interface, spaceInstanceNamespace, spaceName string) bool {
	_, err := managementClient.Loft().ManagementV1().SpaceInstances(spaceInstanceNamespace).Get(context.TODO(), spaceName, metav1.GetOptions{})
	return err == nil
}

func (cmd *SpaceCmd) legacyDeleteSpace(baseClient client.Client, spaceName string) error {
	clusterClient, err := baseClient.Cluster(cmd.Cluster)
	if err != nil {
		return err
	}

	gracePeriod := int64(0)
	err = clusterClient.Agent().ClusterV1().Spaces().Delete(context.TODO(), spaceName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
	if err != nil {
		return errors.Wrap(err, "delete space")
	}

	cmd.Log.Donef("Successfully deleted space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Cluster, "white+b"))

	// update kube config
	if cmd.DeleteContext {
		err = kubeconfig.DeleteContext(kubeconfig.SpaceContextName(cmd.Cluster, spaceName))
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully deleted kube context for space %s", ansi.Color(spaceName, "white+b"))
	}

	// update kube config
	if cmd.Wait {
		cmd.Log.StartWait("Waiting for space to be deleted")
		for isSpaceStillThere(clusterClient, spaceName) {
			time.Sleep(time.Second)
		}
		cmd.Log.StopWait()
		cmd.Log.Done("Space is deleted")
	}

	return nil
}

func isSpaceStillThere(clusterClient kube.Interface, spaceName string) bool {
	_, err := clusterClient.Agent().ClusterV1().Spaces().Get(context.TODO(), spaceName, metav1.GetOptions{})
	return err == nil
}
