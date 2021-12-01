package cmd

import (
	"encoding/json"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/clientauthentication/v1alpha1"
	"os"
)

// TokenCmd holds the cmd flags
type TokenCmd struct {
	*flags.GlobalFlags

	DirectClusterEndpoint bool
	log                   log.Logger
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
			return cmd.Run()
		},
	}

	tokenCmd.Flags().BoolVar(&cmd.DirectClusterEndpoint, "direct-cluster-endpoint", false, "When enabled prints a direct cluster endpoint token")
	return tokenCmd
}

// Run executes the command
func (cmd *TokenCmd) Run() error {
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

	// by default we print the access key as token
	token := config.AccessKey

	// check if we should print a cluster gateway token instead
	if cmd.DirectClusterEndpoint {
		token, err = baseClient.DirectClusterEndpointToken(false)
		if err != nil {
			return err
		}
	}

	return printToken(token)
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
