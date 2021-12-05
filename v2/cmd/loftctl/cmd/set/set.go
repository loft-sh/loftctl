package set

import (
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewSetCmd creates a new cobra command
func NewSetCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
###################### loft set #######################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace set #####################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "set",
		Short: "Set configuration",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewSharedSecretCmd(globalFlags))
	return c
}
