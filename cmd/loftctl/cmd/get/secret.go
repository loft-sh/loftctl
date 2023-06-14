package get

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/set"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	pdefaults "github.com/loft-sh/loftctl/v3/pkg/defaults"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/survey"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/loft-sh/loftctl/v3/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretCmd holds the flags
type SecretCmd struct {
	*flags.GlobalFlags
	Namespace string
	Project   string

	log log.Logger
}

// NewSecretCmd creates a new command
func NewSecretCmd(globalFlags *flags.GlobalFlags, defaults *pdefaults.Defaults) *cobra.Command {
	cmd := &SecretCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################### loft get secret ###################
#######################################################
Returns the key value of a project / shared secret.

Example:
loft get secret test-secret.key
loft get secret test-secret.key --project myproject
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################# devspace get secret #################
#######################################################
Returns the key value of a project / shared secret.

Example:
devspace get secret test-secret.key
devspace get secret test-secret.key --project myproject
#######################################################
	`
	}
	useLine, validator := util.NamedPositionalArgsValidator(true, "SECRET_NAME")
	c := &cobra.Command{
		Use:   "secret" + useLine,
		Short: "Returns the key value of a project / shared secret",
		Long:  description,
		Args:  validator,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(args)
		},
	}

	p, _ := defaults.Get(pdefaults.KeyProject, "")
	c.Flags().StringVarP(&cmd.Project, "project", "p", p, "The project to read the project secret from.")
	c.Flags().StringVarP(&cmd.Namespace, "namespace", "n", "", "The namespace in the loft cluster to read the secret from. If omitted will use the namespace were loft is installed in")
	return c
}

// RunUsers executes the functionality
func (cmd *SecretCmd) Run(args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	var secretType set.SecretType

	if cmd.Project != "" {
		secretType = set.ProjectSecret
	} else {
		secretType = set.SharedSecret
	}

	// get target namespace
	var namespace string

	switch secretType {
	case set.ProjectSecret:
		namespace, err = set.GetProjectSecretNamespace(cmd.Project)
		if err != nil {
			return errors.Wrap(err, "get project secrets namespace")
		}
	case set.SharedSecret:
		namespace, err = set.GetSharedSecretNamespace(cmd.Namespace)
		if err != nil {
			return errors.Wrap(err, "get shared secrets namespace")
		}
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
		secretNameList := []string{}

		switch secretType {
		case set.ProjectSecret:
			secrets, err := managementClient.Loft().ManagementV1().ProjectSecrets(namespace).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return errors.Wrap(err, "list project secrets")
			}

			for _, s := range secrets.Items {
				secretNameList = append(secretNameList, s.Name)
			}
		case set.SharedSecret:
			secrets, err := managementClient.Loft().ManagementV1().SharedSecrets(namespace).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return errors.Wrap(err, "list shared secrets")
			}

			for _, s := range secrets.Items {
				secretNameList = append(secretNameList, s.Name)
			}
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

	var secretData map[string][]byte

	switch secretType {
	case set.ProjectSecret:
		pSecret, err := managementClient.Loft().ManagementV1().ProjectSecrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "get secrets")
		} else if len(pSecret.Spec.Data) == 0 {
			return errors.Errorf("secret %s has no keys to read. Please set a key before trying to read it", secretName)
		}

		secretData = pSecret.Spec.Data
	case set.SharedSecret:
		sSecret, err := managementClient.Loft().ManagementV1().SharedSecrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "get secrets")
		} else if len(sSecret.Spec.Data) == 0 {
			return errors.Errorf("secret %s has no keys to read. Please set a key before trying to read it", secretName)
		}

		secretData = sSecret.Spec.Data
	}

	if keyName == "" {
		keyNames := []string{}

		for k := range secretData {
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

	keyValue, ok := secretData[keyName]
	if !ok {
		return errors.Errorf("key %s does not exist in secret %s", keyName, secretName)
	}

	_, err = os.Stdout.Write(keyValue)
	return err
}
