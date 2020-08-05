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
)

// SleepCmd holds the cmd flags
type SleepCmd struct {
	*flags.GlobalFlags

	Cluster string
	log     log.Logger
}

// NewSleepCmd creates a new command
func NewSleepCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SleepCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	description := `
#######################################################
###################### loft sleep #####################
#######################################################
Sleep puts a space to sleep

Example:
loft sleep myspace
loft sleep myspace --cluster mycluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace sleep ###################
#######################################################
Sleep puts a space to sleep

Example:
devspace sleep myspace
devspace sleep myspace --cluster mycluster
#######################################################
	`
	}

	c := &cobra.Command{
		Use:   "sleep",
		Short: "Put a space to sleep",
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
func (cmd *SleepCmd) Run(cobraCmd *cobra.Command, args []string) error {
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
	sleepModeConfig.Spec.ForceSleep = true

	sleepModeConfig, err = clusterClient.Loft().ClusterV1().SleepModeConfigs(spaceName).Create(context.TODO(), sleepModeConfig, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}
