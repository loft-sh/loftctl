package cmd

import (
	"context"
	"fmt"
	"github.com/ghodss/yaml"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	loftclient "github.com/loft-sh/api/pkg/client/clientset_generated/clientset"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/clihelper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
)

var (
	scheme = runtime.NewScheme()

	_ = clientgoscheme.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)
)

// BackupCmd holds the cmd flags
type BackupCmd struct {
	*flags.GlobalFlags

	Namespace string
	Skip      []string
	Filename  string

	Log log.Logger
}

// NewBackupCmd creates a new command
func NewBackupCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &BackupCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	description := `
#######################################################
##################### loft backup #####################
#######################################################
Backup creates a backup for the Loft management plane

Example:
loft backup
#######################################################
	`

	c := &cobra.Command{
		Use:   "backup",
		Short: "Create a loft management plane backup",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringSliceVar(&cmd.Skip, "skip", []string{}, "What resources the backup should skip. Valid options are: users, teams, accesskeys, sharedsecrets, clusters and clusteraccounttemplates")
	c.Flags().StringVar(&cmd.Namespace, "namespace", "loft", "The namespace to loft was installed into")
	c.Flags().StringVar(&cmd.Filename, "filename", "backup.yaml", "The filename to write the backup to")
	return c
}

// Run executes the functionality
func (cmd *BackupCmd) Run(cobraCmd *cobra.Command, args []string) error {
	// first load the kube config
	kubeClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})

	// load the raw config
	kubeConfig, err := kubeClientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	isInstalled, err := clihelper.IsLoftAlreadyInstalled(kubeClient, cmd.Namespace)
	if err != nil {
		return err
	} else if isInstalled == false {
		return fmt.Errorf("seems like Loft was not installed into namespace %s", cmd.Namespace)
	}

	objects := []runtime.Object{}
	if contains(cmd.Skip, "clusteraccounttemplates") == false {
		cmd.Log.Info("Backing up cluster account templates...")
		objs, err := backupClusterAccountTemplates(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup cluster account templates"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "clusterroletemplates") == false {
		cmd.Log.Info("Backing up clusterroletemplates...")
		objs, err := backupClusterRoles(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup clusterroletemplates"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "clusteraccesses") == false {
		cmd.Log.Info("Backing up clusteraccesses...")
		users, err := backupClusterAccess(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup clusteraccesses"))
		} else {
			objects = append(objects, users...)
		}
	}
	if contains(cmd.Skip, "spaceconstraints") == false {
		cmd.Log.Info("Backing up spaceconstraints...")
		objs, err := backupSpaceConstraints(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup spaceconstraints"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "users") == false {
		cmd.Log.Info("Backing up users...")
		objs, err := backupUsers(kubeClient, kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup users"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "teams") == false {
		cmd.Log.Info("Backing up teams...")
		objs, err := backupTeams(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup teams"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "sharedsecrets") == false {
		cmd.Log.Info("Backing up shared secrets...")
		objs, err := backupSharedSecrets(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup shared secrets"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "accesskeys") == false {
		cmd.Log.Info("Backing up access keys...")
		objs, err := backupAccessKeys(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup access keys"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "apps") == false {
		cmd.Log.Info("Backing up apps...")
		objs, err := backupApps(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup apps"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "spacetemplates") == false {
		cmd.Log.Info("Backing up space templates...")
		objs, err := backupSpaceTemplates(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup space templates"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "virtualclustertemplates") == false {
		cmd.Log.Info("Backing up virtual cluster templates...")
		objs, err := backupVirtualClusterTemplate(kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup virtual cluster templates"))
		} else {
			objects = append(objects, objs...)
		}
	}
	if contains(cmd.Skip, "clusters") == false {
		cmd.Log.Info("Backing up clusters...")
		objs, err := backupClusters(kubeClient, kubeConfig)
		if err != nil {
			cmd.Log.Warn(errors.Wrap(err, "backup clusters"))
		} else {
			objects = append(objects, objs...)
		}
	}

	// create a file
	retString := []string{}
	for _, o := range objects {
		out, err := yaml.Marshal(o)
		if err != nil {
			return errors.Wrap(err, "marshal object")
		}

		retString = append(retString, string(out))
	}

	cmd.Log.Infof("Writing backup to %s...", cmd.Filename)
	err = ioutil.WriteFile(cmd.Filename, []byte(strings.Join(retString, "\n---\n")), 0644)
	if err != nil {
		return err
	}

	cmd.Log.Donef("Wrote backup to %s", cmd.Filename)
	return nil
}

func backupClusters(kubeClient kubernetes.Interface, rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	clusterList, err := loftClient.StorageV1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range clusterList.Items {
		u := o
		u.Status = storagev1.ClusterStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)

		// find user secrets
		if u.Spec.Config.SecretName != "" {
			secret, err := getSecret(kubeClient, u.Spec.Config.SecretNamespace, u.Spec.Config.SecretName)
			if err != nil {
				return nil, errors.Wrap(err, "get cluster secret")
			} else if secret != nil {
				retList = append(retList, secret)
			}
		}
	}

	return retList, nil
}

func backupClusterRoles(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	objs, err := loftClient.StorageV1().ClusterRoleTemplates().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range objs.Items {
		u := o
		u.Status = storagev1.ClusterRoleTemplateStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupSpaceConstraints(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	objs, err := loftClient.StorageV1().SpaceConstraints().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range objs.Items {
		u := o
		u.Status = storagev1.SpaceConstraintStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupClusterAccess(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	objs, err := loftClient.StorageV1().ClusterAccesses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range objs.Items {
		u := o
		u.Status = storagev1.ClusterAccessStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupClusterAccountTemplates(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	clusterAccountTemplateList, err := loftClient.StorageV1().ClusterAccountTemplates().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range clusterAccountTemplateList.Items {
		u := o
		u.Status = storagev1.ClusterAccountTemplateStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupVirtualClusterTemplate(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	apps, err := loftClient.StorageV1().VirtualClusterTemplates().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range apps.Items {
		u := o
		u.Status = storagev1.VirtualClusterTemplateStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupSpaceTemplates(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	apps, err := loftClient.StorageV1().SpaceTemplates().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range apps.Items {
		u := o
		u.Status = storagev1.SpaceTemplateStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupApps(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	apps, err := loftClient.StorageV1().Apps().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range apps.Items {
		u := o
		u.Status = storagev1.AppStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupAccessKeys(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	accessKeyList, err := loftClient.StorageV1().AccessKeys().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, o := range accessKeyList.Items {
		u := o
		u.Status = storagev1.AccessKeyStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupSharedSecrets(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	sharedSecretList, err := loftClient.StorageV1().SharedSecrets("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, sharedSecret := range sharedSecretList.Items {
		u := sharedSecret
		u.Status = storagev1.SharedSecretStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupTeams(rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	teamList, err := loftClient.StorageV1().Teams().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, team := range teamList.Items {
		u := team
		u.Status = storagev1.TeamStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)
	}

	return retList, nil
}

func backupUsers(kubeClient kubernetes.Interface, rest *rest.Config) ([]runtime.Object, error) {
	loftClient, err := loftclient.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	userList, err := loftClient.StorageV1().Users().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retList := []runtime.Object{}
	for _, user := range userList.Items {
		u := user
		u.Status = storagev1.UserStatus{}
		err := resetMetadata(&u)
		if err != nil {
			return nil, err
		}

		retList = append(retList, &u)

		// find user secrets
		if u.Spec.PasswordRef != nil {
			secret, err := getSecret(kubeClient, u.Spec.PasswordRef.SecretNamespace, u.Spec.PasswordRef.SecretName)
			if err != nil {
				return nil, errors.Wrap(err, "get user secret")
			} else if secret != nil {
				retList = append(retList, secret)
			}
		}
		if u.Spec.CodesRef != nil {
			secret, err := getSecret(kubeClient, u.Spec.CodesRef.SecretNamespace, u.Spec.CodesRef.SecretName)
			if err != nil {
				return nil, errors.Wrap(err, "get user secret")
			} else if secret != nil {
				retList = append(retList, secret)
			}
		}
	}

	return retList, nil
}

func getSecret(kubeClient kubernetes.Interface, namespace, name string) (*corev1.Secret, error) {
	if namespace == "" || name == "" {
		return nil, nil
	}

	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return nil, err
	} else if secret != nil {
		err = resetMetadata(secret)
		if err != nil {
			return nil, errors.Wrap(err, "reset metadata secret")
		}

		return secret, nil
	}

	return nil, nil
}

func resetMetadata(obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	accessor.SetGenerateName("")
	accessor.SetSelfLink("")
	accessor.SetClusterName("")
	accessor.SetCreationTimestamp(metav1.Time{})
	accessor.SetFinalizers(nil)
	accessor.SetGeneration(0)
	accessor.SetManagedFields(nil)
	accessor.SetOwnerReferences(nil)
	accessor.SetResourceVersion("")
	accessor.SetUID("")
	accessor.SetDeletionTimestamp(nil)

	gvk, err := GVKFrom(obj)
	if err != nil {
		return err
	}

	typeAccessor, err := meta.TypeAccessor(obj)
	if err != nil {
		return err
	}

	typeAccessor.SetKind(gvk.Kind)
	typeAccessor.SetAPIVersion(gvk.GroupVersion().String())
	return nil
}

func contains(arr []string, s string) bool {
	for _, t := range arr {
		if t == s {
			return true
		}
	}
	return false
}

func GVKFrom(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	} else if len(gvks) != 1 {
		return schema.GroupVersionKind{}, fmt.Errorf("unexpected number of object kinds: %d", len(gvks))
	}

	return gvks[0], nil
}
