package cmd

import (
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/connect"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/create"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/delete"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/generate"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/get"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/importcmd"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/list"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/reset"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/set"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/share"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/sleep"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/use"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/vars"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/wakeup"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewRootCmd returns a new root command
func NewRootCmd(log log.Logger) *cobra.Command {
	return &cobra.Command{
		Use:           "loft",
		SilenceUsage:  true,
		SilenceErrors: true,
		Short:         "Welcome to Loft!",
		PersistentPreRun: func(cobraCmd *cobra.Command, args []string) {
			if globalFlags.Silent {
				log.SetLevel(logrus.FatalLevel)
			}
		},
		Long: `Loft CLI - www.loft.sh`,
	}
}

var globalFlags *flags.GlobalFlags

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	log := log.GetInstance()
	rootCmd := BuildRoot(log)

	// Set version for --version flag
	rootCmd.Version = upgrade.GetVersion()

	// Execute command
	err := rootCmd.Execute()
	if err != nil {
		if globalFlags.Debug {
			log.Fatalf("%+v", err)
		} else {
			log.Fatal(err)
		}
	}
}

// BuildRoot creates a new root command from the
func BuildRoot(log log.Logger) *cobra.Command {
	rootCmd := NewRootCmd(log)
	persistentFlags := rootCmd.PersistentFlags()
	globalFlags = flags.SetGlobalFlags(persistentFlags)

	// add top level commands
	rootCmd.AddCommand(NewStartCmd(globalFlags))
	rootCmd.AddCommand(NewLoginCmd(globalFlags))
	rootCmd.AddCommand(NewTokenCmd(globalFlags))
	rootCmd.AddCommand(NewBackupCmd(globalFlags))
	rootCmd.AddCommand(NewCompletionCmd(rootCmd, globalFlags))
	rootCmd.AddCommand(NewUpgradeCmd())

	// add subcommands
	rootCmd.AddCommand(list.NewListCmd(globalFlags))
	rootCmd.AddCommand(use.NewUseCmd(globalFlags))
	rootCmd.AddCommand(create.NewCreateCmd(globalFlags))
	rootCmd.AddCommand(delete.NewDeleteCmd(globalFlags))
	rootCmd.AddCommand(generate.NewGenerateCmd(globalFlags))
	rootCmd.AddCommand(get.NewGetCmd(globalFlags))
	rootCmd.AddCommand(vars.NewVarsCmd(globalFlags))
	rootCmd.AddCommand(share.NewShareCmd(globalFlags))
	rootCmd.AddCommand(set.NewSetCmd(globalFlags))
	rootCmd.AddCommand(reset.NewResetCmd(globalFlags))
	rootCmd.AddCommand(sleep.NewSleepCmd(globalFlags))
	rootCmd.AddCommand(wakeup.NewWakeUpCmd(globalFlags))
	rootCmd.AddCommand(importcmd.NewImportCmd(globalFlags))
	rootCmd.AddCommand(connect.NewConnectCmd(globalFlags))

	return rootCmd
}
