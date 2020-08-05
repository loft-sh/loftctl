package connect

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
)

// NewConnectCmd creates a new cobra command
func NewConnectCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	description := `
#######################################################
#################### loft connect #####################
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################## devspace connect ###################
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "connect",
		Short: "Connects to loft resources",
		Long:  description,
		Args:  cobra.NoArgs,
	}

	c.AddCommand(NewVirtualClusterCmd(globalFlags))
	return c
}
