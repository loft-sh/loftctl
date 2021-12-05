package list

import (
	"context"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"strings"
	"time"
)

// SharedSecretsCmd holds the cmd flags
type SharedSecretsCmd struct {
	*flags.GlobalFlags
	Namespace string

	log log.Logger
}

// NewSharedSecretsCmd creates a new command
func NewSharedSecretsCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SharedSecretsCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################## loft list secrets ##################
#######################################################
List the shared secrets you have access to

Example:
loft list secrets
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace list secrets ################
#######################################################
List the shared secrets you have access to

Example:
devspace list secrets
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "secrets",
		Short: "List all the shared secrets you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVarP(&cmd.Namespace, "namespace", "n", "", "The namespace in the loft cluster to read the secret from. If omitted will query all accessible secrets")
	return c
}

// Run executes the functionality
func (cmd *SharedSecretsCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	client, err := baseClient.Management()
	if err != nil {
		return err
	}

	secrets, err := client.Loft().ManagementV1().SharedSecrets(cmd.Namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	header := []string{
		"Name",
		"Namespace",
		"Keys",
		"Age",
	}
	values := [][]string{}
	for _, secret := range secrets.Items {
		keyNames := []string{}
		for k := range secret.Spec.Data {
			keyNames = append(keyNames, k)
		}

		values = append(values, []string{
			secret.Name,
			secret.Namespace,
			strings.Join(keyNames, ","),
			duration.HumanDuration(time.Now().Sub(secret.CreationTimestamp.Time)),
		})
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
