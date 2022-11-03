package wakeup

import (
	"context"
	"fmt"
	clusterv1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/cluster/v1"
	managementv1 "github.com/loft-sh/api/v2/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/pkg/client/naming"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"strconv"
	"time"

	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	Project string
	Cluster string
	Log     log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
################### loft wakeup space #################
#######################################################
wakeup resumes a sleeping space

Example:
loft wakeup space myspace
loft wakeup space myspace --project myproject
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace wakeup space ################
#######################################################
wakeup resumes a sleeping space

Example:
devspace wakeup space myspace
devspace wakeup space myspace --project myproject
#######################################################
	`
	}

	c := &cobra.Command{
		Use:   "space",
		Short: "Wakes up a space",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(args)
		},
	}

	c.Flags().StringVarP(&cmd.Project, "project", "p", "", "The project to use")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	return c
}

// Run executes the functionality
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
		return cmd.legacySpaceWakeUp(baseClient, spaceName)
	}

	return cmd.spaceWakeUp(baseClient, spaceName)
}

func (cmd *SpaceCmd) spaceWakeUp(baseClient client.Client, spaceName string) error {
	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	spaceInstance, err := managementClient.Loft().ManagementV1().SpaceInstances(naming.ProjectNamespace(cmd.Project)).Get(context.TODO(), spaceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if spaceInstance.Annotations == nil {
		spaceInstance.Annotations = map[string]string{}
	}
	delete(spaceInstance.Annotations, clusterv1.SleepModeForceAnnotation)
	delete(spaceInstance.Annotations, clusterv1.SleepModeForceDurationAnnotation)
	spaceInstance.Annotations[clusterv1.SleepModeLastActivityAnnotation] = strconv.FormatInt(time.Now().Unix(), 10)

	_, err = managementClient.Loft().ManagementV1().SpaceInstances(naming.ProjectNamespace(cmd.Project)).Update(context.TODO(), spaceInstance, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// wait for sleeping
	cmd.Log.StartWait("Wait until space wakes up")
	defer cmd.Log.StopWait()
	err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
		spaceInstance, err := managementClient.Loft().ManagementV1().SpaceInstances(naming.ProjectNamespace(cmd.Project)).Get(context.TODO(), spaceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return spaceInstance.Status.Phase != storagev1.InstanceSleeping, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for space to wake up: %v", err)
	}

	cmd.Log.Donef("Successfully woken up space %s", spaceName)
	return nil
}

func (cmd *SpaceCmd) legacySpaceWakeUp(baseClient client.Client, spaceName string) error {
	clusterClient, err := baseClient.Cluster(cmd.Cluster)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get current user / team
	self, err := managementClient.Loft().ManagementV1().Selves().Create(context.TODO(), &managementv1.Self{}, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "get self")
	} else if self.Status.User == nil && self.Status.Team == nil {
		return fmt.Errorf("no user or team name returned")
	}

	configs, err := clusterClient.Agent().ClusterV1().SleepModeConfigs(spaceName).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	sleepModeConfig := &configs.Items[0]
	sleepModeConfig.Spec.ForceSleep = false
	sleepModeConfig.Spec.ForceSleepDuration = nil
	sleepModeConfig.Status.LastActivity = time.Now().Unix()

	sleepModeConfig, err = clusterClient.Agent().ClusterV1().SleepModeConfigs(spaceName).Create(context.TODO(), sleepModeConfig, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	// wait for sleeping
	cmd.Log.StartWait("Wait until space wakes up")
	defer cmd.Log.StopWait()
	err = wait.Poll(time.Second, time.Minute, func() (bool, error) {
		configs, err := clusterClient.Agent().ClusterV1().SleepModeConfigs(spaceName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		return configs.Items[0].Status.SleepingSince == 0, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for space to wake up: %v", err)
	}

	cmd.Log.Donef("Successfully woken up space %s", spaceName)
	return nil
}
