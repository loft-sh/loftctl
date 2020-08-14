package delete

import (
	"context"
	"time"

	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	Cluster       string
	DeleteContext bool
	Wait          bool

	log log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################# loft delete space ###################
#######################################################
Deletes a space from a cluster

Example:
loft delete space 
loft delete space myspace
loft delete space myspace --cluster mycluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace delete space #################
#######################################################
Deletes a space from a cluster

Example:
devspace delete space 
devspace delete space myspace
devspace delete space myspace --cluster mycluster
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "space",
		Short: "Deletes a space from a cluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().BoolVar(&cmd.DeleteContext, "delete-context", true, "If the corresponding kube context should be deleted if there is any")
	c.Flags().BoolVar(&cmd.Wait, "wait", false, "Termination of this command waits for space to be deleted")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	spaceName := ""
	if len(args) > 0 {
		spaceName = args[0]
	}

	spaceName, clusterName, err := helper.SelectSpaceAndClusterName(baseClient, spaceName, cmd.Cluster, cmd.log)
	if err != nil {
		return err
	}

	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	gracePeriod := int64(0)
	err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Delete(context.TODO(), spaceName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
	if err != nil {
		return errors.Wrap(err, "delete space")
	}

	cmd.log.Donef("Successfully deleted space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// update kube config
	if cmd.DeleteContext {
		err = kubeconfig.DeleteContext(kubeconfig.ContextName(clusterName, spaceName))
		if err != nil {
			return err
		}

		cmd.log.Donef("Successfully deleted kube context for space %s", ansi.Color(spaceName, "white+b"))
	}

	// update kube config
	if cmd.Wait {
		cmd.log.StartWait("Waiting for space to be deleted")
		for isSpaceStillThere(clusterClient, spaceName) {
			time.Sleep(time.Second)
		}
		cmd.log.StopWait()
		cmd.log.Done("Space is deleted")
	}

	return nil
}

func isSpaceStillThere(clusterClient kube.Interface, spaceName string) bool {
	_, err := clusterClient.Kiosk().TenancyV1alpha1().Spaces().Get(context.TODO(), spaceName, metav1.GetOptions{})
	return err == nil
}
