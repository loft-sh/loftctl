package sleep

import (
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewSleepCmd creates a new cobra command
func NewSleepCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft sleep ######################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################### devspace sleep ####################
#######################################################
	`
	}
	cmd := &cobra.Command{
		Use:   "sleep",
		Short: "Puts spaces or vclusters to sleep",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(NewSpaceCmd(globalFlags))
	cmd.AddCommand(NewVClusterCmd(globalFlags))
	return cmd
}
