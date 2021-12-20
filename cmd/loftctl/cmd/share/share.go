package share

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewShareCmd creates a new cobra command
func NewShareCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
##################### loft share ######################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################### devspace share ####################
#######################################################
	`
	}
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Shares cluster resources",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(NewSpaceCmd(globalFlags))
	cmd.AddCommand(NewVClusterCmd(globalFlags))
	return cmd
}
