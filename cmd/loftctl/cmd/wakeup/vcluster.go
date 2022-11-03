package wakeup

import (
	"context"
	"fmt"
	clusterv1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/cluster/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/client/naming"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"strconv"
	"time"
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

	virtualClusterInstance, err := managementClient.Loft().ManagementV1().VirtualClusterInstances(naming.ProjectNamespace(cmd.Project)).Get(context.TODO(), vClusterName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if virtualClusterInstance.Annotations == nil {
		virtualClusterInstance.Annotations = map[string]string{}
	}
	delete(virtualClusterInstance.Annotations, clusterv1.SleepModeForceAnnotation)
	delete(virtualClusterInstance.Annotations, clusterv1.SleepModeForceDurationAnnotation)
	virtualClusterInstance.Annotations[clusterv1.SleepModeLastActivityAnnotation] = strconv.FormatInt(time.Now().Unix(), 10)

	_, err = managementClient.Loft().ManagementV1().VirtualClusterInstances(naming.ProjectNamespace(cmd.Project)).Update(context.TODO(), virtualClusterInstance, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// wait for sleeping
	cmd.Log.StartWait("Wait until virtual cluster wakes up")
	defer cmd.Log.StopWait()
	err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
		virtualClusterInstance, err := managementClient.Loft().ManagementV1().VirtualClusterInstances(naming.ProjectNamespace(cmd.Project)).Get(context.TODO(), vClusterName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return virtualClusterInstance.Status.Phase != storagev1.InstanceSleeping, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for vcluster to wake up: %v", err)
	}

	cmd.Log.Donef("Successfully woken up vcluster %s", vClusterName)
	return nil
}
