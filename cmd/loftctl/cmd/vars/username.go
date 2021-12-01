package vars

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
)

type usernameCmd struct {
	*flags.GlobalFlags
}

func newUsernameCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &usernameCmd{
		GlobalFlags: globalFlags,
	}

	return &cobra.Command{
		Use:   "username",
		Short: "Prints the current loft username",
		Args:  cobra.NoArgs,
		RunE:  cmd.Run,
	}
}

// Run executes the command logic
func (cmd *usernameCmd) Run(cobraCmd *cobra.Command, args []string) error {
	retError := fmt.Errorf("Not logged in loft, but predefined var LOFT_USERNAME is used.")
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return retError
	}

	client, err := baseClient.Management()
	if err != nil {
		return err
	}

	userName, teamName, err := helper.GetCurrentUser(context.TODO(), client)
	if err != nil {
		return err
	} else if teamName != nil {
		return errors.New("logged in with a team and not a user")
	}

	_, err = os.Stdout.Write([]byte(userName.Username))
	return err
}
