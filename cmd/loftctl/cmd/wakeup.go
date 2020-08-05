package cmd

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// WakeUpCmd holds the cmd flags
type WakeUpCmd struct {
	*flags.GlobalFlags

	Cluster string
	log     log.Logger
}

// NewWakeUpCmd creates a new command
func NewWakeUpCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &WakeUpCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	description := `
#######################################################
###################### loft wakeup ####################
#######################################################
wakeup resumes a sleeping space

Example:
loft wakeup myspace
loft wakeup myspace --cluster mycluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################### devspace wakeup ###################
#######################################################
wakeup resumes a sleeping space

Example:
devspace wakeup myspace
devspace wakeup myspace --cluster mycluster
#######################################################
	`
	}

	c := &cobra.Command{
		Use:   "wakeup",
		Short: "Wakes up a space",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	return c
}

// Run executes the functionality
func (cmd *WakeUpCmd) Run(cobraCmd *cobra.Command, args []string) error {
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

	configs, err := clusterClient.Loft().ClusterV1().SleepModeConfigs(spaceName).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	sleepModeConfig := &configs.Items[0]
	sleepModeConfig.Spec.ForceSleep = false
	sleepModeConfig.Status.LastActivity = time.Now().Unix()

	sleepModeConfig, err = clusterClient.Loft().ClusterV1().SleepModeConfigs(spaceName).Create(context.TODO(), sleepModeConfig, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}
