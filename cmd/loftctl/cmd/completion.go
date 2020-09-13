package cmd

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/log"
	"os"

	"github.com/spf13/cobra"
)

// CompletionCmd holds the cmd flags
type CompletionCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

func NewCompletionCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &CompletionCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `To load completions:

Bash:

$ source <(yourprogram completion bash)

# To load completions for each session, execute once:
Linux:
  $ yourprogram completion bash > /etc/bash_completion.d/yourprogram
MacOS:
  $ yourprogram completion bash > /usr/local/etc/bash_completion.d/yourprogram

Zsh:

# If shell completion is not already enabled in your environment you will need
# to enable it.  You can execute the following once:

$ echo "autoload -U compinit; compinit" >> ~/.zshrc

# To load completions for each session, execute once:
$ yourprogram completion zsh > "${fpath[1]}/_yourprogram"

# You will need to start a new shell for this setup to take effect.

Fish:

$ yourprogram completion fish | source

# To load completions for each session, execute once:
$ yourprogram completion fish > ~/.config/fish/completions/yourprogram.fish
	`

	// completionCmd represents the completion command
	var completionCmd = &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate completion script",
		Long:                  description,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}
	return completionCmd
}

func (cmd *CompletionCmd) Run(cobraCmd *cobra.Command, args []string) error {
	switch args[0] {
	case "bash":
		return cobraCmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		return cobraCmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		return cobraCmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		return cobraCmd.Root().GenPowerShellCompletion(os.Stdout)
	}
	return nil
}
