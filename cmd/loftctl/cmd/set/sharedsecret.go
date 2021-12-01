package set

import (
	"context"
	managementv1 "github.com/loft-sh/api/v2/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/survey"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// SharedSecretCmd holds the lags
type SharedSecretCmd struct {
	*flags.GlobalFlags
	Namespace string

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
################### loft set secret ###################
#######################################################
Sets the key value of a shared secret

Example:
loft set secret test-secret.key value
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################# devspace set secret #################
#######################################################
Sets the key value of a shared secret

Example:
devspace set secret test-secret.key value
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "secret [secret.key] [value]",
		Short: "Sets the key value of a shared secret",
		Long:  description,
		Args:  cobra.ExactArgs(2),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVarP(&cmd.Namespace, "namespace", "n", "", "The namespace in the loft cluster to create the secret in. If omitted will use the namespace were loft is installed in")
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
	secretArg := args[0]
	idx := strings.Index(secretArg, ".")
	if idx == -1 {
		secretName = secretArg
	} else {
		secretName = secretArg[:idx]
		keyName = secretArg[idx+1:]
	}

	// get target namespace
	namespace, err := GetSharedSecretNamespace(cmd.Namespace)
	if err != nil {
		return errors.Wrap(err, "get shared secrets namespace")
	}

	secret, err := managementClient.Loft().ManagementV1().SharedSecrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) == false {
			return errors.Wrap(err, "get secret")
		}

		secret = nil
	}

	if keyName == "" {
		if secret == nil {
			return errors.Errorf("please specify a secret key to set. For example 'set secret my-secret.key value'")
		}
		if len(secret.Spec.Data) == 0 {
			return errors.Errorf("secret %s has no keys. Please specify a key like `loft set secret name.key value`", secretName)
		}

		keyNames := []string{}
		for k := range secret.Spec.Data {
			keyNames = append(keyNames, k)
		}

		keyName, err = cmd.log.Question(&survey.QuestionOptions{
			Question:     "Please select a secret key to set",
			DefaultValue: keyNames[0],
			Options:      keyNames,
		})
		if err != nil {
			return errors.Wrap(err, "ask question")
		}
	}

	// create the secret
	if secret == nil {
		_, err = managementClient.Loft().ManagementV1().SharedSecrets(namespace).Create(context.TODO(), &managementv1.SharedSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
			},
			Spec: managementv1.SharedSecretSpec{
				SharedSecretSpec: storagev1.SharedSecretSpec{
					Data: map[string][]byte{
						keyName: []byte(args[1]),
					},
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		cmd.log.Donef("Successfully created secret %s with key %s", secretName, keyName)
		return nil
	}

	// Update the secret
	if secret.Spec.Data == nil {
		secret.Spec.Data = map[string][]byte{}
	}
	secret.Spec.Data[keyName] = []byte(args[1])
	_, err = managementClient.Loft().ManagementV1().SharedSecrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "update secret")
	}

	cmd.log.Donef("Successfully set secret key %s.%s", secretName, keyName)
	return nil
}

func GetSharedSecretNamespace(namespace string) (string, error) {
	if namespace == "" {
		namespace = "loft"
	}

	return namespace, nil
}
