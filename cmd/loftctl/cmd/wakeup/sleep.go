package wakeup

import (
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewWakeUpCmd creates a new cobra command
func NewWakeUpCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft wakeup #####################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################### devspace wakeup ###################
#######################################################
	`
	}
	cmd := &cobra.Command{
		Use:   "wakeup",
		Short: "Wakes up a space or vcluster",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(NewSpaceCmd(globalFlags))
	cmd.AddCommand(NewVClusterCmd(globalFlags))
	return cmd
}
