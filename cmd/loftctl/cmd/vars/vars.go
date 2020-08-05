package vars

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/spf13/cobra"
)

// NewVarsCmd creates a new cobra command for the sub command
func NewVarsCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "Print predefined variables",
		Long: `
#######################################################
################### devspace vars #####################
#######################################################
	`,
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(newUsernameCmd(globalFlags))
	cmd.AddCommand(newClusterCmd(globalFlags))
	return cmd
}
