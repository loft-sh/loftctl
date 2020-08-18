package cmd

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	"strings"
)

// LoginCmd holds the login cmd flags
type LoginCmd struct {
	*flags.GlobalFlags

	Username  string
	AccessKey string
	Insecure  bool

	Log log.Logger
}

// NewLoginCmd creates a new open command
func NewLoginCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &LoginCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
###################### loft login #####################
#######################################################
Login into loft

Example:
loft login https://my-loft.com
loft login https://my-loft.com --username myuser --access-key myaccesskey
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace login ###################
#######################################################
Login into loft

Example:
devspace login https://my-loft.com
devspace login https://my-loft.com --username myuser --access-key myaccesskey
#######################################################
	`
	}
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to a loft instance",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.RunLogin(cobraCmd, args)
		},
	}

	loginCmd.Flags().StringVar(&cmd.Username, "username", "", "The username to use")
	loginCmd.Flags().StringVar(&cmd.AccessKey, "access-key", "", "The access key to use")
	loginCmd.Flags().BoolVar(&cmd.Insecure, "insecure", false, "Allow login into an insecure loft instance")
	return loginCmd
}

// RunLogin executes the functionality "loft login"
func (cmd *LoginCmd) RunLogin(cobraCmd *cobra.Command, args []string) error {
	loader, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// Print login information
	if len(args) == 0 {
		config := loader.Config()
		if config.Host == "" {
			cmd.Log.Info("Not logged in")
			return nil
		}

		cmd.Log.Infof("Logged into %s as %s", config.Host, config.Username)
		return nil
	}

	url := args[0]
	if strings.HasPrefix(url, "http") == false {
		url = "https://" + url
	}

	// log into loft
	if cmd.Username != "" && cmd.AccessKey != "" {
		err = loader.LoginWithAccessKey(url, cmd.Username, cmd.AccessKey, cmd.Insecure)
	} else {
		err = loader.Login(url, cmd.Insecure, cmd.Log)
	}

	if err != nil {
		return err
	}

	cmd.Log.Donef("Successfully logged into loft at %s", ansi.Color(url, "white+b"))
	return nil
}
