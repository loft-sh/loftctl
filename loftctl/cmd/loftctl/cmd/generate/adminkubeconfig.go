package generate

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"time"
)

// AdminKubeConfigCmd holds the cmd flags
type AdminKubeConfigCmd struct {
	*flags.GlobalFlags

	Namespace      string
	ServiceAccount string
	log            log.Logger
}

// NewAdminKubeConfigCmd creates a new command
func NewAdminKubeConfigCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &AdminKubeConfigCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
########## loft generate admin-kube-config ############
#######################################################
Creates a new kube config that can be used to connect
a cluster to loft.

Example:
loft generate admin-kube-config
loft generate admin-kube-config --namespace mynamespace
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
######## devspace generate admin-kube-config ##########
#######################################################
Creates a new kube config that can be used to connect
a cluster to loft.

Example:
devspace generate admin-kube-config
devspace generate admin-kube-config --namespace mynamespace
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "admin-kube-config",
		Short: "Generates a new kube config for connecting a cluster",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Namespace, "namespace", "loft", "The namespace to generate the service account in. The namespace will be created if it does not exist")
	c.Flags().StringVar(&cmd.ServiceAccount, "service-account", "loft", "The service account name to create")
	return c
}

// Run executes the command
func (cmd *AdminKubeConfigCmd) Run(cobraCmd *cobra.Command, args []string) error {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	c, err := loader.ClientConfig()
	if err != nil {
		return err
	}

	client, err := kubernetes.NewForConfig(c)
	if err != nil {
		return errors.Wrap(err, "create kube client")
	}

	// make sure namespace exists
	_, err = client.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: cmd.Namespace,
		},
	}, v1.CreateOptions{})
	if err != nil {
		if kerrors.IsAlreadyExists(err) == false {
			return err
		}
	}

	// create service account
	_, err = client.CoreV1().ServiceAccounts(cmd.Namespace).Create(context.TODO(), &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name: cmd.ServiceAccount,
		},
	}, v1.CreateOptions{})
	if err != nil {
		if kerrors.IsAlreadyExists(err) == false {
			return err
		}
	}

	// create clusterrolebinding
	client.RbacV1().ClusterRoleBindings().Create(context.TODO(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name: cmd.ServiceAccount + "-binding",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      cmd.ServiceAccount,
				Namespace: cmd.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}, v1.CreateOptions{})

	// wait for secret
	token := []byte{}
	err = wait.Poll(time.Millisecond*250, time.Minute*2, func() (bool, error) {
		serviceAccount, err := client.CoreV1().ServiceAccounts(cmd.Namespace).Get(context.TODO(), cmd.ServiceAccount, v1.GetOptions{})
		if err != nil {
			return false, errors.Wrap(err, "retrieve service account")
		} else if len(serviceAccount.Secrets) == 0 {
			return false, nil
		}

		secret, err := client.CoreV1().Secrets(cmd.Namespace).Get(context.TODO(), serviceAccount.Secrets[0].Name, v1.GetOptions{})
		if err != nil {
			return false, errors.Wrap(err, "get service account secret")
		}

		ok := false
		token, ok = secret.Data["token"]
		if !ok {
			return false, fmt.Errorf("service account secret has unexpected contents")
		}

		return true, nil
	})
	if err != nil {
		return err
	}

	// print kube config
	return kubeconfig.PrintTokenKubeConfig(c, string(token))
}
