package create

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewCreateCmd creates a new cobra command
func NewCreateCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft create #####################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
##################### loft create #####################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "create",
		Short: "Creates loft resources",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewSpaceCmd(globalFlags))
	c.AddCommand(NewVirtualClusterCmd(globalFlags))
	return c
}
