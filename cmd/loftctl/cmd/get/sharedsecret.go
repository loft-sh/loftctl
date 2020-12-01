package get

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
)

// SharedSecretCmd holds the lags
type SharedSecretCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewSharedSecretCmd creates a new command
func NewSharedSecretCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SharedSecretCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################### loft get secret ###################
#######################################################
Returns the key value of a shared secret.

Example:
loft get secret test-secret.key
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################# devspace get secret #################
#######################################################
Returns the key value of a shared secret.

Example:
devspace get secret test-secret.key
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "secret",
		Short: "Returns the key value of a shared secret",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return c
}

// RunUsers executes the functionality
func (cmd *SharedSecretCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get secret
	secretName := ""
	keyName := ""
	if len(args) == 1 {
		secret := args[0]
		idx := strings.Index(secret, ".")
		if idx == -1 {
			secretName = secret
		} else {
			secretName = secret[:idx]
			keyName = secret[idx+1:]
		}
	} else {
		secrets, err := managementClient.Loft().ManagementV1().SharedSecrets().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "list shared secrets")
		}

		secretNameList := []string{}
		for _, s := range secrets.Items {
			secretNameList = append(secretNameList, s.Name)
		}

		if len(secretNameList) == 0 {
			return fmt.Errorf("couldn't find any secrets that could be read. Please make sure to create a shared secret before you try to read it")
		}

		secretName, err = cmd.log.Question(&survey.QuestionOptions{
			Question:     "Please select a secret to read from",
			DefaultValue: secretNameList[0],
			Options:      secretNameList,
		})
		if err != nil {
			return errors.Wrap(err, "ask question")
		}
	}

	secret, err := managementClient.Loft().ManagementV1().SharedSecrets().Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "get secrets")
	} else if len(secret.Spec.Data) == 0 {
		return errors.Errorf("secret %s has no keys to read. Please set a key before trying to read it", secretName)
	}

	if keyName == "" {
		keyNames := []string{}
		for k := range secret.Spec.Data {
			keyNames = append(keyNames, k)
		}

		keyName, err = cmd.log.Question(&survey.QuestionOptions{
			Question:     "Please select a secret key to read",
			DefaultValue: keyNames[0],
			Options:      keyNames,
		})
		if err != nil {
			return errors.Wrap(err, "ask question")
		}
	}

	keyValue, ok := secret.Spec.Data[keyName]
	if !ok {
		return errors.Errorf("key %s does not exist in secret %s", keyName, secretName)
	}

	_, err = os.Stdout.Write(keyValue)
	return err
}
