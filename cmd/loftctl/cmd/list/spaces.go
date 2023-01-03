package list

import (
	storagev1 "github.com/loft-sh/api/v3/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/helper"
	"github.com/loft-sh/loftctl/v3/pkg/clihelper"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/duration"
	"strconv"
	"time"
)

// SpacesCmd holds the login cmd flags
type SpacesCmd struct {
	*flags.GlobalFlags

	ShowLegacy bool

	log log.Logger
}

// NewSpacesCmd creates a new spaces command
func NewSpacesCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpacesCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################## loft list spaces ###################
#######################################################
List the loft spaces you have access to

Example:
loft list spaces
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace list spaces #################
#######################################################
List the loft spaces you have access to

Example:
devspace list spaces
#######################################################
	`
	}
	listCmd := &cobra.Command{
		Use:   "spaces",
		Short: "Lists the loft spaces you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.RunSpaces()
		},
	}
	listCmd.Flags().BoolVar(&cmd.ShowLegacy, "show-legacy", false, "If true, will always show the legacy spaces as well")
	return listCmd
}

// RunSpaces executes the functionality
func (cmd *SpacesCmd) RunSpaces() error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	header := []string{
		"Name",
		"Project",
		"Cluster",
		"Sleeping",
		"Status",
		"Age",
	}
	values := [][]string{}
	spaceInstances, err := helper.GetSpaceInstances(baseClient)
	if err != nil {
		return err
	}
	for _, space := range spaceInstances {
		values = append(values, []string{
			clihelper.GetDisplayName(space.SpaceInstance.Name, space.SpaceInstance.Spec.DisplayName),
			space.Project,
			space.SpaceInstance.Spec.ClusterRef.Cluster,
			strconv.FormatBool(space.SpaceInstance.Status.Phase == storagev1.InstanceSleeping),
			string(space.SpaceInstance.Status.Phase),
			duration.HumanDuration(time.Now().Sub(space.SpaceInstance.CreationTimestamp.Time)),
		})
	}
	if len(spaceInstances) == 0 || cmd.ShowLegacy {
		spaces, err := helper.GetSpaces(baseClient, cmd.log)
		if err != nil {
			return err
		}
		for _, space := range spaces {
			sleepModeConfig := space.Status.SleepModeConfig
			sleeping := "false"
			if sleepModeConfig.Status.SleepingSince != 0 {
				sleeping = duration.HumanDuration(time.Now().Sub(time.Unix(sleepModeConfig.Status.SleepingSince, 0)))
			}
			spaceName := space.Name
			if space.Annotations != nil && space.Annotations["loft.sh/display-name"] != "" {
				spaceName = space.Annotations["loft.sh/display-name"] + " (" + spaceName + ")"
			}

			values = append(values, []string{
				spaceName,
				"",
				space.Cluster,
				sleeping,
				string(space.Space.Status.Phase),
				duration.HumanDuration(time.Now().Sub(space.Space.CreationTimestamp.Time)),
			})
		}
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
