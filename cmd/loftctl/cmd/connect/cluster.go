package connect

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/helper"
	"github.com/loft-sh/loftctl/v3/pkg/config"
	"github.com/loft-sh/loftctl/v3/pkg/kube"
	"github.com/loft-sh/log"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v3/pkg/apis/storage/v1"
	"github.com/loft-sh/api/v3/pkg/product"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/generate"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type ClusterCmd struct {
	*flags.GlobalFlags
	Log            log.Logger
	Namespace      string
	ServiceAccount string
	DisplayName    string
	Wait           bool
	Experimental   bool
}

// NewClusterCmd creates a new command
func NewClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ClusterCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := product.ReplaceWithHeader("connect cluster", `
Connect a cluster to the Loft instance.

Example:
loft connect cluster my-cluster
########################################################
	`)
	if upgrade.IsPlugin == "true" {
		description = `
########################################################
################ devspace connect cluster ##############
########################################################
Connect a cluster to the Loft instance.

Example:
devspace connect cluster my-cluster
########################################################
	`
	}
	c := &cobra.Command{
		Use:   "cluster",
		Short: product.Replace("connect current cluster to Loft"),
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
			c, err := loader.ClientConfig()
			if err != nil {
				return fmt.Errorf("get kube config: %w", err)
			}

			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd.Context(), c, args)
		},
	}

	c.Flags().StringVar(&cmd.Namespace, "namespace", "loft", "The namespace to generate the service account in. The namespace will be created if it does not exist")
	c.Flags().StringVar(&cmd.ServiceAccount, "service-account", "loft-admin", "The service account name to create")
	c.Flags().StringVar(&cmd.DisplayName, "display-name", "", "The display name to show in the UI for this cluster")
	c.Flags().BoolVar(&cmd.Wait, "wait", false, "If true, will wait until the cluster is initialized")
	c.Flags().BoolVar(&cmd.Experimental, "experimental", false, "If true, will use a new, experimental, egress-only cluster enrollment feature")

	return c
}

func (cmd *ClusterCmd) Run(ctx context.Context, c *rest.Config, args []string) error {
	// Get clusterName from command argument
	clusterName := args[0]

	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return fmt.Errorf("new client from path: %w", err)
	}

	err = client.VerifyVersion(baseClient)
	if err != nil {
		return fmt.Errorf("verify loft version: %w", err)
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return fmt.Errorf("create management client: %w", err)
	}

	// get user details
	user, team, err := getUserOrTeam(ctx, managementClient)
	if err != nil {
		return fmt.Errorf("get user or team: %w", err)
	}

	if cmd.Experimental {
		_, err = managementClient.Loft().ManagementV1().Clusters().Create(ctx, &managementv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterName,
			},
			Spec: managementv1.ClusterSpec{
				ClusterSpec: storagev1.ClusterSpec{
					DisplayName: cmd.DisplayName,
					Owner: &storagev1.UserOrTeam{
						User: user,
						Team: team,
					},
					NetworkPeer: true,
					Access:      getAccess(user, team),
				},
			},
		}, metav1.CreateOptions{})
		if err != nil && !kerrors.IsAlreadyExists(err) {
			return fmt.Errorf("create cluster: %w", err)
		}

		accessKey, err := managementClient.Loft().ManagementV1().Clusters().GetAccessKey(ctx, clusterName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get cluster access key: %w", err)
		}

		args := []string{
			"upgrade", "--install", "loft", "loft",
			"--repo", "https://charts.loft.sh",
			"--create-namespace",
			"--namespace", "loft",
			"--set", "agentOnly=true",
		}

		if os.Getenv("DEVELOPMENT") == "true" {
			args = []string{
				"upgrade", "--install", "loft", "./chart",
				"--create-namespace",
				"--namespace", "loft",
				"--set", "agentOnly=true",
				"--set", "image=ghcr.io/loft-sh/enterprise:release-test",
			}
		}

		if accessKey.LoftHost != "" {
			args = append(args, "--set", "url="+accessKey.LoftHost)
		}

		if accessKey.AccessKey != "" {
			args = append(args, "--set", "token="+accessKey.AccessKey)
		}

		if accessKey.Insecure {
			args = append(args, "--set", "insecureSkipVerify=true")
		}

		if accessKey.CaCert != "" {
			args = append(args, "--set", "additionalCA="+accessKey.CaCert)
		}

		if cmd.Wait {
			args = append(args, "--wait")
		}

		helmCmd := exec.CommandContext(ctx, "helm", args...)
		helmCmd.Stdout = os.Stdout
		helmCmd.Stderr = os.Stderr
		helmCmd.Stdin = os.Stdin

		err = helmCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to install loft chart: %w", err)
		}
	} else {
		// get cluster config
		clusterConfig, err := getClusterKubeConfig(ctx, c, cmd.Namespace, cmd.ServiceAccount)
		if err != nil {
			return fmt.Errorf("get cluster kubeconfig: %w", err)
		}

		// connect cluster
		_, err = managementClient.Loft().ManagementV1().ClusterConnects().Create(ctx, &managementv1.ClusterConnect{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterName,
			},
			Spec: managementv1.ClusterConnectSpec{
				Config:    clusterConfig.String(),
				AdminUser: user,
				ClusterTemplate: managementv1.Cluster{
					Spec: managementv1.ClusterSpec{
						ClusterSpec: storagev1.ClusterSpec{
							DisplayName: cmd.DisplayName,
							Owner: &storagev1.UserOrTeam{
								User: user,
								Team: team,
							},
							Access: getAccess(user, team),
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create cluster connect: %w", err)
		}
	}

	if cmd.Wait {
		cmd.Log.Info("Waiting for the cluster to be initialized...")
		waitErr := wait.PollUntilContextTimeout(ctx, time.Second, config.Timeout(), false, func(ctx context.Context) (done bool, err error) {
			clusterInstance, err := managementClient.Loft().ManagementV1().Clusters().Get(ctx, clusterName, metav1.GetOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				return false, err
			}

			return clusterInstance.Status.Phase == storagev1.ClusterStatusPhaseInitialized, nil
		})
		if waitErr != nil {
			return errors.Wrap(waitErr, "get cluster")
		}
	}

	cmd.Log.Donef("Successfully connected cluster %s to Loft", clusterName)

	return nil
}

func getUserOrTeam(ctx context.Context, managementClient kube.Interface) (string, string, error) {
	var user, team string

	userName, teamName, err := helper.GetCurrentUser(ctx, managementClient)
	if err != nil {
		return "", "", fmt.Errorf("get current user: %w", err)
	}

	if userName != nil {
		user = userName.Name
	} else {
		team = teamName.Name
	}

	return user, team, nil
}

func getAccess(user, team string) []storagev1.Access {
	access := []storagev1.Access{
		{
			Verbs:        []string{"*"},
			Subresources: []string{"*"},
		},
	}

	if team != "" {
		access[0].Teams = []string{team}
	} else {
		access[0].Users = []string{user}
	}

	return access
}

func getClusterKubeConfig(ctx context.Context, c *rest.Config, namespace, serviceAccount string) (bytes.Buffer, error) {
	var clusterConfig bytes.Buffer

	token, err := generate.GetAuthToken(ctx, c, namespace, serviceAccount)
	if err != nil {
		return bytes.Buffer{}, errors.Wrap(err, "get auth token")
	}

	err = kubeconfig.WriteTokenKubeConfig(c, string(token), &clusterConfig)
	if err != nil {
		return bytes.Buffer{}, errors.Wrap(err, "write token kubeconfig")
	}

	return clusterConfig, nil
}
