package get

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"time"
)

// UserCmd holds the lags
type UserCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewUserCmd creates a new command
func NewUserCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &UserCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
#################### loft get user ####################
#######################################################
Returns the currently logged in user

Example:
loft get user
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################## devspace get user ##################
#######################################################
Returns the currently logged in user

Example:
devspace get user
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "user",
		Short: "Retrieves the current logged in user",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return c
}

// RunUsers executes the functionality
func (cmd *UserCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	client, err := baseClient.Management()
	if err != nil {
		return err
	}

	userName, teamName, err := helper.GetCurrentUser(context.TODO(), client)
	if err != nil {
		return err
	} else if teamName != "" {
		return errors.New("logged in with a team and not a user")
	}

	user, err := client.Loft().ManagementV1().Users().Get(context.TODO(), userName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "list users")
	}

	header := []string{
		"Username",
		"Kubernetes Name",
		"Email",
		"Age",
	}
	values := [][]string{}
	values = append(values, []string{
		user.Spec.Username,
		user.Name,
		user.Spec.Email,
		duration.HumanDuration(time.Now().Sub(user.CreationTimestamp.Time)),
	})

	log.PrintTable(cmd.log, header, values)
	return nil
}
