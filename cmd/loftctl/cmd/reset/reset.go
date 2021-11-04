package reset

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewResetCmd creates a new cobra command
func NewResetCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft reset ######################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################### devspace reset ####################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewPasswordCmd(globalFlags))
	return c
}
