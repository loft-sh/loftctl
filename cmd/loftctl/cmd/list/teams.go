package list

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"strconv"
	"time"
)

// TeamsCmd holds the login cmd flags
type TeamsCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewTeamsCmd creates a new spaces command
func NewTeamsCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &TeamsCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
#################### loft list teams ##################
#######################################################
List the loft teams you are member of

Example:
loft list teams
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################## devspace list teams ################
#######################################################
List the loft teams you are member of

Example:
devspace list teams
#######################################################
	`
	}
	clustersCmd := &cobra.Command{
		Use:   "teams",
		Short: "Lists the loft teams you are member of",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return clustersCmd
}

// RunUsers executes the functionality "loft list users"
func (cmd *TeamsCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	authInfo, err := baseClient.AuthInfo()
	if err != nil {
		return err
	}

	client, err := baseClient.Management()
	if err != nil {
		return err
	}

	teams, err := client.Loft().ManagementV1().Users().ListTeams(context.TODO(), authInfo.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	header := []string{
		"Name",
		"Users",
		"Groups",
		"Age",
	}
	values := [][]string{}
	for _, team := range teams.Teams {
		values = append(values, []string{
			team.Name,
			strconv.Itoa(len(team.Spec.Users)),
			strconv.Itoa(len(team.Spec.Groups)),
			duration.HumanDuration(time.Now().Sub(team.CreationTimestamp.Time)),
		})
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
