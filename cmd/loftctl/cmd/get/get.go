package get

import (
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewGetCmd creates a new cobra command
func NewGetCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
###################### loft get #######################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace get #####################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "get",
		Short: "Get configuration",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewUserCmd(globalFlags))
	c.AddCommand(NewSharedSecretCmd(globalFlags))
	return c
}
