package cmd

import (
	"encoding/json"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/clientauthentication/v1alpha1"
	"os"
)

// TokenCmd holds the cmd flags
type TokenCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewTokenCmd creates a new command
func NewTokenCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &TokenCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	description := `
#######################################################
###################### loft token #####################
#######################################################
Prints an access token to a loft instance. This can
be used as an ExecAuthenticator for kubernetes

Example:
loft token
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace token ###################
#######################################################
Prints an access token to a loft instance. This can
be used as an ExecAuthenticator for kubernetes

Example:
devspace token
#######################################################
	`
	}

	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Token prints the access token to a loft instance",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return tokenCmd
}

// Run executes the command
func (cmd *TokenCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}
	
	// get config
	config := baseClient.Config()
	if config == nil {
		return errors.New("no config loaded")
	} else if config.Host == "" || config.AccessKey == "" {
		return errors.New("not logged in, please make sure you have run 'loft login [loft-url]'")
	}
	
	return printToken(config.AccessKey)
}

func printToken(token string) error {
	// Print token to stdout
	response := &v1alpha1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExecCredential",
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		Status: &v1alpha1.ExecCredentialStatus{
			Token: token,
		},
	}

	bytes, err := json.Marshal(response)
	if err != nil {
		return errors.Wrap(err, "json marshal")
	}

	_, err = os.Stdout.Write(bytes)
	return err
}
