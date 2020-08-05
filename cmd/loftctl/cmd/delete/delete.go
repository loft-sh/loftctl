package delete

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewDeleteCmd creates a new cobra command
func NewDeleteCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft delete #####################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
##################### loft delete #####################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "delete",
		Short: "Deletes loft resources",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewSpaceCmd(globalFlags))
	c.AddCommand(NewVirtualClusterCmd(globalFlags))
	return c
}
