package generate

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewGenerateCmd creates a new cobra command
func NewGenerateCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
#################### loft generate ####################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################## devspace generate ##################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "generate",
		Short: "Generates configuration",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewAdminKubeConfigCmd(globalFlags))
	return c
}
