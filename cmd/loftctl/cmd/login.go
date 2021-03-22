package cmd

import (
	"bytes"
	"context"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/docker"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// LoginCmd holds the login cmd flags
type LoginCmd struct {
	*flags.GlobalFlags

	Username  string
	AccessKey string
	Insecure  bool
	
	DockerLogin bool
	Log log.Logger
}

// NewLoginCmd creates a new open command
func NewLoginCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &LoginCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
###################### loft login #####################
#######################################################
Login into loft

Example:
loft login https://my-loft.com
loft login https://my-loft.com --access-key myaccesskey
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
#################### devspace login ###################
#######################################################
Login into loft

Example:
devspace login https://my-loft.com
devspace login https://my-loft.com --access-key myaccesskey
#######################################################
	`
	}
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to a loft instance",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.RunLogin(cobraCmd, args)
		},
	}

	loginCmd.Flags().StringVar(&cmd.Username, "username", "", "DEPRECATED DO NOT USE ANYMORE")
	loginCmd.Flags().StringVar(&cmd.AccessKey, "access-key", "", "The access key to use")
	loginCmd.Flags().BoolVar(&cmd.Insecure, "insecure", false, "Allow login into an insecure loft instance")
	loginCmd.Flags().BoolVar(&cmd.DockerLogin, "docker-login", true, "If true, will log into the docker image registries the user has image pull secrets for")
	return loginCmd
}

// RunLogin executes the functionality "loft login"
func (cmd *LoginCmd) RunLogin(cobraCmd *cobra.Command, args []string) error {
	if cmd.Username != "" {
		cmd.Log.Warnf("--username is deprecated, please do not use anymore and only use --access-key")
	}
	
	loader, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// Print login information
	if len(args) == 0 {
		config := loader.Config()
		if config.Host == "" {
			cmd.Log.Info("Not logged in")
			return nil
		}
		
		managementClient, err := loader.Management()
		if err != nil {
			return err
		}

		userName, err := helper.GetCurrentUser(context.TODO(), managementClient)
		if err != nil {
			return err
		}

		cmd.Log.Infof("Logged into %s as %s", config.Host, userName)
		return nil
	}

	url := args[0]
	if strings.HasPrefix(url, "http") == false {
		url = "https://" + url
	}

	// log into loft
	url = strings.TrimSuffix(url, "/")
	if cmd.AccessKey != "" {
		err = loader.LoginWithAccessKey(url, cmd.AccessKey, cmd.Insecure)
	} else {
		err = loader.Login(url, cmd.Insecure, cmd.Log)
	}
	if err != nil {
		return err
	}
	cmd.Log.Donef("Successfully logged into loft at %s", ansi.Color(url, "white+b"))
	
	// skip log into docker registries?
	if cmd.DockerLogin == false {
		return nil
	}
	
	err = dockerLogin(loader, cmd.Log)
	if err != nil {
		return err
	}
	
	return nil
}

func dockerLogin(loader client.Client, log log.Logger) error {
	managementClient, err := loader.Management()
	if err != nil {
		return err
	}

	// get user name
	userName, err := helper.GetCurrentUser(context.TODO(), managementClient)
	if err != nil {
		return err
	}

	dockerConfigs := []*configfile.ConfigFile{}

	// get image pull secrets from teams
	teams, err := managementClient.Loft().ManagementV1().Users().ListTeams(context.TODO(), userName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, team := range teams.Teams {
		dockerConfigs = append(dockerConfigs, collectImagePullSecrets(context.TODO(), managementClient, team.Spec.ImagePullSecrets, log)...)
	}
	
	// get image pull secrets from user
	user, err := managementClient.Loft().ManagementV1().Users().Get(context.TODO(), userName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	dockerConfigs = append(dockerConfigs, collectImagePullSecrets(context.TODO(), managementClient, user.Spec.ImagePullSecrets, log)...)
	
	// store docker configs
	if len(dockerConfigs) > 0 {
		dockerConfig, err := docker.NewDockerConfig()
		if err != nil {
			return err
		}
		
		// log into registries locally
		for _, config := range dockerConfigs {
			for registry, authConfig := range config.AuthConfigs {
				err = dockerConfig.Store(registry, authConfig)
				if err != nil {
					return err
				}

				if registry == "https://index.docker.io/v1/" {
					registry = "docker hub"
				}

				log.Donef("Successfully logged into docker registry '%s'", registry)
			}
		}
		
		err = dockerConfig.Save()
		if err != nil {
			return errors.Wrap(err, "save docker config")
		}
	}
	
	return nil
}

func collectImagePullSecrets(ctx context.Context, managementClient kube.Interface, imagePullSecrets []*storagev1.KindSecretRef, log log.Logger) []*configfile.ConfigFile {
	retConfigFiles := []*configfile.ConfigFile{}
	for _, imagePullSecret := range imagePullSecrets {
		// unknown image pull secret type?
		if imagePullSecret.Kind != "SharedSecret" || (imagePullSecret.APIGroup != storagev1.SchemeGroupVersion.Group && imagePullSecret.APIGroup != managementv1.SchemeGroupVersion.Group) {
			continue
		} else if imagePullSecret.SecretName == "" || imagePullSecret.SecretNamespace == "" {
			continue
		}
		
		sharedSecret, err := managementClient.Loft().ManagementV1().SharedSecrets(imagePullSecret.SecretNamespace).Get(ctx, imagePullSecret.SecretName, metav1.GetOptions{})
		if err != nil {
			log.Warnf("Unable to retrieve image pull secret %s/%s: %v", imagePullSecret.SecretNamespace, imagePullSecret.SecretName, err)
			continue
		} else if len(sharedSecret.Spec.Data) == 0 {
			log.Warnf("Unable to retrieve image pull secret %s/%s: secret is empty", imagePullSecret.SecretNamespace, imagePullSecret.SecretName)
			continue
		} else if imagePullSecret.Key == "" && len(sharedSecret.Spec.Data) > 1 {
			log.Warnf("Unable to retrieve image pull secret %s/%s: secret has multiple keys, but none is specified for image pull secret", imagePullSecret.SecretNamespace, imagePullSecret.SecretName)
			continue
		}
		
		// determine shared secret key
		key := imagePullSecret.Key
		if key == "" {
			for k := range sharedSecret.Spec.Data {
				key = k
			}
		}

		configFile, err := dockerconfig.LoadFromReader(bytes.NewReader(sharedSecret.Spec.Data[key]))
		if err != nil {
			log.Warnf("Parsing image pull secret %s/%s.%s: %v", imagePullSecret.SecretNamespace, imagePullSecret.SecretName, key, err)
			continue
		}

		retConfigFiles = append(retConfigFiles, configFile)
	}
	
	return retConfigFiles
}

