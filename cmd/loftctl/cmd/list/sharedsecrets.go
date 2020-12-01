package list

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"strings"
	"time"
)

// SharedSecretsCmd holds the cmd flags
type SharedSecretsCmd struct {
	*flags.GlobalFlags

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
	clustersCmd := &cobra.Command{
		Use:   "secrets",
		Short: "List the shared secrets you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return clustersCmd
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

	secrets, err := client.Loft().ManagementV1().SharedSecrets().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	header := []string{
		"Name",
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
			strings.Join(keyNames, ","),
			duration.HumanDuration(time.Now().Sub(secret.CreationTimestamp.Time)),
		})
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
