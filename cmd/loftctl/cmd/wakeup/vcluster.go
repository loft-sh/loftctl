package wakeup

import (
	"context"
	"fmt"

	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/helper"
	"github.com/loft-sh/loftctl/v3/pkg/client/naming"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/loft-sh/loftctl/v3/pkg/vcluster"
	"github.com/spf13/cobra"
)

// VClusterCmd holds the cmd flags
type VClusterCmd struct {
	*flags.GlobalFlags

	Project string

	Log log.Logger
}

// NewVClusterCmd creates a new command
func NewVClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VClusterCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
################# loft wakeup vcluster ################
#######################################################
Wakes up a vcluster

Example:
loft wakeup vcluster myvcluster
loft wakeup vcluster myvcluster --project myproject
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############## devspace wakeup vcluster ###############
#######################################################
Wakes up a vcluster

Example:
devspace wakeup vcluster myvcluster
devspace wakeup vcluster myvcluster --project myproject
#######################################################
	`
	}

	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Wake up a vcluster",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(args)
		},
	}

	c.Flags().StringVarP(&cmd.Project, "project", "p", "", "The project to use")
	return c
}

// Run executes the functionality
func (cmd *VClusterCmd) Run(args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	vClusterName := ""
	if len(args) > 0 {
		vClusterName = args[0]
	}

	_, cmd.Project, _, vClusterName, err = helper.SelectVirtualClusterInstanceOrVirtualCluster(baseClient, vClusterName, "", cmd.Project, "", cmd.Log)
	if err != nil {
		return err
	}

	if cmd.Project == "" {
		return fmt.Errorf("couldn't find a vcluster you have access to")
	}

	return cmd.wakeUpVCluster(baseClient, vClusterName)
}

func (cmd *VClusterCmd) wakeUpVCluster(baseClient client.Client, vClusterName string) error {
	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	_, err = vcluster.WaitForVirtualClusterInstance(context.TODO(), managementClient, naming.ProjectNamespace(cmd.Project), vClusterName, true, cmd.Log)
	if err != nil {
		return err
	}

	return nil
}
