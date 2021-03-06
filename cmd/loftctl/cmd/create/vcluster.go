package create

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/cmd/use"
	"io"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"strconv"
	"strings"
	"time"

	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	tenancyv1alpha1 "github.com/loft-sh/kiosk/pkg/apis/tenancy/v1alpha1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	SleepAfter    int64
	DeleteAfter   int64
	Image         string
	Cluster       string
	Space         string
	Account       string
	CreateContext bool
	SwitchContext bool
	Print         bool

	DisableDirectClusterEndpoint bool

	Out io.Writer
	Log log.Logger
}

// NewVirtualClusterCmd creates a new command
func NewVirtualClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VirtualClusterCmd{
		GlobalFlags: globalFlags,
		Out:         os.Stdout,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
################ loft create vcluster #################
#######################################################
Creates a new virtual cluster in a given space and
cluster. If no space or cluster is specified the user 
will be asked.

Example:
loft create vcluster test
loft create vcluster test --cluster mycluster
loft create vcluster test --cluster mycluster --space myspace
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############## devspace create vcluster ###############
#######################################################
Creates a new virtual cluster in a given space and
cluster. If no space or cluster is specified the user 
will be asked.

Example:
devspace create vcluster test
devspace create vcluster test --cluster mycluster
devspace create vcluster test --cluster mycluster --space myspace
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Creates a new virtual cluster in the given parent cluster",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to create the virtual cluster in")
	c.Flags().StringVar(&cmd.Space, "space", "", "The space to create the virtual cluster in")
	c.Flags().StringVar(&cmd.Account, "account", "", "The cluster account to create the space with if it doesn't exist")
	c.Flags().BoolVar(&cmd.Print, "print", false, "If enabled, prints the context to the console")
	c.Flags().Int64Var(&cmd.SleepAfter, "sleep-after", 0, "If set to non zero, will tell the space to sleep after specified seconds of inactivity")
	c.Flags().Int64Var(&cmd.DeleteAfter, "delete-after", 0, "If set to non zero, will tell loft to delete the space after specified seconds of inactivity")
	c.Flags().BoolVar(&cmd.CreateContext, "create-context", true, "If loft should create a kube context for the space")
	c.Flags().BoolVar(&cmd.SwitchContext, "switch-context", true, "If loft should switch the current context to the new context")
	c.Flags().BoolVar(&cmd.DisableDirectClusterEndpoint, "disable-direct-cluster-endpoint", false, "When enabled does not use an available direct cluster endpoint to connect to the vcluster")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(cobraCmd *cobra.Command, args []string) error {
	ctx := context.Background()
	virtualClusterName := args[0]
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// determine cluster name
	if cmd.Cluster == "" {
		cmd.Cluster, err = helper.SelectCluster(baseClient, cmd.Log)
		if err != nil {
			return err
		}
	}

	// determine space name
	if cmd.Space == "" {
		cmd.Space = "vcluster-" + virtualClusterName
	}

	// create a cluster client
	clusterClient, err := baseClient.Cluster(cmd.Cluster)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}
	defaults, err := managementClient.Loft().ManagementV1().Clusters().ListVirtualClusterDefaults(ctx, cmd.Cluster, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if defaults.Warning != "" {
		warningLines := strings.Split(defaults.Warning, "\n")
		for _, w := range warningLines {
			cmd.Log.Warn(w)
		}
	}

	// check if the cluster exists
	cluster, err := managementClient.Loft().ManagementV1().Clusters().Get(context.TODO(), cmd.Cluster, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			return fmt.Errorf("cluster '%s' does not exist, or you don't have permission to use it", cmd.Cluster)
		}

		return err
	}

	// create space if it does not exist
	_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Get(ctx, cmd.Space, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) == false {
			return err
		}

		// determine account name
		accountName := cmd.Account
		if accountName == "" {
			accountName, err = helper.SelectAccount(baseClient, cmd.Cluster, cmd.Log)
			if err != nil {
				return err
			}
		}

		// get owner references
		ownerReferences, err := getOwnerReferences(managementClient, cmd.Cluster, accountName)
		if err != nil {
			return err
		}

		// space object
		space := &tenancyv1alpha1.Space{
			ObjectMeta: metav1.ObjectMeta{
				Name:            cmd.Space,
				Annotations:     map[string]string{},
				OwnerReferences: ownerReferences,
			},
			Spec: tenancyv1alpha1.SpaceSpec{
				Account: accountName,
			},
		}
		if cmd.SleepAfter > 0 {
			space.Annotations["sleepmode.loft.sh/sleep-after"] = strconv.FormatInt(cmd.SleepAfter, 10)
		}
		if cmd.DeleteAfter > 0 {
			space.Annotations["sleepmode.loft.sh/delete-after"] = strconv.FormatInt(cmd.DeleteAfter, 10)
		}

		// create the space
		_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Create(ctx, space, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "create space")
		}
	}

	// create the virtual cluster
	secretName := "vc-" + virtualClusterName
	_, err = clusterClient.Loft().StorageV1().VirtualClusters(cmd.Space).Create(ctx, &storagev1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualClusterName,
			Namespace: cmd.Space,
		},
		Spec: storagev1.VirtualClusterSpec{
			HelmRelease: &storagev1.VirtualClusterHelmRelease{
				Chart: storagev1.VirtualClusterHelmChart{
					Version: defaults.LatestVersion,
				},
				Values: defaults.Values,
			},
			Pod: &storagev1.PodSelector{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"release": virtualClusterName,
					},
				},
			},
			KubeConfigRef: &storagev1.SecretRef{
				SecretName:      secretName,
				SecretNamespace: cmd.Space,
				Key:             "config",
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create vcluster")
	}

	cmd.Log.Donef("Successfully created the virtual cluster %s in cluster %s and space %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Cluster, "white+b"), ansi.Color(cmd.Space, "white+b"))

	// should we create a kube context for the virtual context
	if cmd.CreateContext || cmd.Print {
		vClusterClient, err := baseClient.VirtualCluster(cmd.Cluster, cmd.Space, virtualClusterName)
		if err != nil {
			return err
		}

		// wait until virtual cluster is reachable
		cmd.Log.StartWait("Waiting for vCluster to be reachable...")
		err = wait.PollImmediate(time.Second, time.Minute*5, func() (bool, error) {
			_, err := vClusterClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
		cmd.Log.StopWait()
		if err != nil {
			return err
		}

		// create kube context options
		contextOptions, err := use.CreateVClusterContextOptions(baseClient, cmd.Config, cluster, cmd.Space, virtualClusterName, cmd.DisableDirectClusterEndpoint, cmd.SwitchContext, cmd.Log)
		if err != nil {
			return err
		}

		// check if we should print the config
		if cmd.Print {
			err = kubeconfig.PrintKubeConfigTo(contextOptions, cmd.Out)
			if err != nil {
				return err
			}
		}

		// check if we should update the config
		if cmd.CreateContext {
			// update kube config
			err = kubeconfig.UpdateKubeConfig(contextOptions)
			if err != nil {
				return err
			}

			cmd.Log.Donef("Successfully updated kube context to use virtual cluster %s in space %s and cluster %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Space, "white+b"), ansi.Color(cmd.Cluster, "white+b"))
		}
	}

	return nil
}
