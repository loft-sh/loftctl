package devpod

import (
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/spf13/cobra"
)

// NewDevPodCmd creates a new cobra command
func NewDevPodCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	c := &cobra.Command{
		Use:    "devpod",
		Hidden: true,
		Short:  "DevPod commands",
		Long: `
#######################################################
##################### loft devpod #####################
#######################################################
	`,
		Args: cobra.NoArgs,
	}

	c.AddCommand(NewUpCmd(globalFlags))
	return c
}
