package create

import (
	"context"
	"fmt"
	agentstoragev1 "github.com/loft-sh/agentapi/pkg/apis/loft/storage/v1"
	v1 "github.com/loft-sh/api/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	virtualclusterv1 "github.com/loft-sh/api/pkg/apis/virtualcluster/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/cmd/use"
	"github.com/loft-sh/loftctl/pkg/kube"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"strconv"
	"strings"
	"time"

	tenancyv1alpha1 "github.com/loft-sh/agentapi/pkg/apis/kiosk/tenancy/v1alpha1"
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
	Template      string
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
	c.Flags().StringVar(&cmd.Template, "template", "", "The virtual cluster template to use to create the virtual cluster")
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
	
	var (
		vClusterValues string
		vClusterVersion string
		vClusterTemplate *storagev1.VirtualClusterTemplateSpec
		vClusterTemplateName string
	)
	if cmd.Template == "" {
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
		
		vClusterValues = defaults.Values
		vClusterVersion = defaults.LatestVersion
		if defaults.DefaultTemplate != nil {
			vClusterTemplate = &defaults.DefaultTemplate.Spec
			vClusterTemplateName = defaults.DefaultTemplate.Name
		}
	} else {
		virtualClusterTemplate, err := managementClient.Loft().ManagementV1().VirtualClusterTemplates().Get(ctx, cmd.Template, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if virtualClusterTemplate.Spec.Template.HelmRelease != nil {
			vClusterValues = virtualClusterTemplate.Spec.Template.HelmRelease.Values
			vClusterVersion = virtualClusterTemplate.Spec.Template.HelmRelease.Chart.Version
		}
		vClusterTemplate = &virtualClusterTemplate.Spec.VirtualClusterTemplateSpec
		vClusterTemplateName = virtualClusterTemplate.Name
	}
	
	// resolve apps
	var vClusterApps []namespacedApp
	if vClusterTemplate != nil && len(vClusterTemplate.Template.Apps) > 0 {
		vClusterApps, err = resolveVClusterApps(managementClient, vClusterTemplate.Template.Apps)
		if err != nil {
			return errors.Wrap(err, "resolve virtual cluster template apps")
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
	spaceCreated, err := cmd.createSpace(ctx, baseClient, clusterClient, managementClient, vClusterTemplate, cluster)
	if err != nil {
		return errors.Wrap(err, "create space")
	}
	
	// create the object
	secretName := "vc-" + virtualClusterName
	vCluster := &agentstoragev1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualClusterName,
			Namespace: cmd.Space,
		},
		Spec: agentstoragev1.VirtualClusterSpec{
			HelmRelease: &agentstoragev1.VirtualClusterHelmRelease{
				Chart: agentstoragev1.VirtualClusterHelmChart{
					Version: vClusterVersion,
				},
				Values: vClusterValues,
			},
			Pod: &agentstoragev1.PodSelector{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"release": virtualClusterName,
					},
				},
			},
			KubeConfigRef: &agentstoragev1.SecretRef{
				SecretName:      secretName,
				SecretNamespace: cmd.Space,
				Key:             "config",
			},
		},
	}
	if vClusterTemplate != nil {
		cmd.Log.Infof("Using virtual cluster template %s", vClusterTemplateName)
		vCluster.Annotations = vClusterTemplate.Template.Metadata.Annotations
		vCluster.Labels = vClusterTemplate.Template.Metadata.Labels
	}

	// create the virtual cluster
	_, err = clusterClient.Agent().StorageV1().VirtualClusters(cmd.Space).Create(ctx, vCluster, metav1.CreateOptions{})
	if err != nil {
		if spaceCreated {
			_ = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Delete(context.TODO(), cmd.Space, metav1.DeleteOptions{})
		}
		
		return errors.Wrap(err, "create virtual cluster")
	}
	cleanup := func() {
		if spaceCreated {
			_ = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Delete(context.TODO(), cmd.Space, metav1.DeleteOptions{})
		} else {
			_ = clusterClient.Agent().StorageV1().VirtualClusters(cmd.Space).Delete(ctx, vCluster.Name, metav1.DeleteOptions{})
		}
	}
	
	// create a vcluster client
	vClusterClient, err := baseClient.VirtualCluster(cmd.Cluster, cmd.Space, virtualClusterName)
	if err != nil {
		cleanup()
		return err
	}
	
	// create the virtual cluster template instances
	if len(vClusterApps) > 0 {
		// wait until virtual cluster is reachable
		cmd.Log.StartWait("Waiting for virtual cluster to be reachable...")
		err = wait.PollImmediate(time.Second, time.Minute*10, func() (bool, error) {
			_, err := vClusterClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
		cmd.Log.StopWait()
		if err != nil {
			cleanup()
			return errors.Wrap(err, "waiting for virtual cluster to become reachable")
		}
		
		// deploy the apps
		for _, app := range vClusterApps {
			cmd.Log.Infof("Deploying app %s into virtual cluster namespace %s...", app.App.Name, app.Namespace)
			_, err := vClusterClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: app.Namespace},
			}, metav1.CreateOptions{})
			if err != nil && kerrors.IsAlreadyExists(err) == false {
				cleanup()
				return errors.Wrap(err, "create namespace")
			}

			release := convertAppToHelmRelease(app.App)
			vRelease := &virtualclusterv1.HelmRelease{
				ObjectMeta: release.ObjectMeta,
				Spec: virtualclusterv1.HelmReleaseSpec{
					HelmReleaseSpec: release.Spec,
				},
			}
			
			_, err = vClusterClient.Loft().VirtualclusterV1().HelmReleases(app.Namespace).Create(context.TODO(), vRelease, metav1.CreateOptions{})
			if err != nil {
				cleanup()
				return errors.Wrap(err, "deploy app " + app.App.Name)
			}
		}
	}

	cmd.Log.Donef("Successfully created the virtual cluster %s in cluster %s and space %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Cluster, "white+b"), ansi.Color(cmd.Space, "white+b"))

	// should we create a kube context for the virtual context
	if cmd.CreateContext || cmd.Print {
		// wait until virtual cluster is reachable
		cmd.Log.StartWait("Waiting for virtual cluster to be reachable...")
		err = wait.PollImmediate(time.Second, time.Minute*5, func() (bool, error) {
			_, err := vClusterClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
		cmd.Log.StopWait()
		if err != nil {
			cleanup()
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

func (cmd *VirtualClusterCmd) createSpace(ctx context.Context, baseClient client.Client, clusterClient kube.Interface, managementClient kube.Interface, vClusterTemplate *storagev1.VirtualClusterTemplateSpec, cluster *v1.Cluster) (bool, error) {
	_, err := clusterClient.Kiosk().TenancyV1alpha1().Spaces().Get(ctx, cmd.Space, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) == false {
			return false, err
		}

		// determine account name
		accountName := cmd.Account
		if accountName == "" {
			accountName, err = helper.SelectAccount(baseClient, cmd.Cluster, cmd.Log)
			if err != nil {
				return false, err
			}
		}

		// get owner references
		ownerReferences, err := getOwnerReferences(managementClient, cmd.Cluster, accountName)
		if err != nil {
			return false, err
		}

		// resolve space template
		template := ""
		if vClusterTemplate != nil && vClusterTemplate.SpaceTemplateRef != nil {
			template = vClusterTemplate.SpaceTemplateRef.Name
		}

		// get space template
		spaceTemplate, err := resolveSpaceTemplate(managementClient, cluster, template)
		if err != nil {
			return false, errors.Wrap(err, "resolve space template")
		} else if spaceTemplate != nil {
			cmd.Log.Infof("Using space template %s to create space %s", spaceTemplate.Name, cmd.Space)
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
		if spaceTemplate != nil {
			space.Annotations = spaceTemplate.Spec.Template.Metadata.Annotations
			space.Labels = spaceTemplate.Spec.Template.Metadata.Labels
		}
		if cmd.SleepAfter > 0 {
			space.Annotations["sleepmode.loft.sh/sleep-after"] = strconv.FormatInt(cmd.SleepAfter, 10)
		}
		if cmd.DeleteAfter > 0 {
			space.Annotations["sleepmode.loft.sh/delete-after"] = strconv.FormatInt(cmd.DeleteAfter, 10)
		}

		// resolve the space apps
		var apps []v1.App
		if spaceTemplate != nil && len(spaceTemplate.Spec.Template.Apps) > 0 {
			apps, err = resolveApps(managementClient, spaceTemplate.Spec.Template.Apps)
			if err != nil {
				return false, errors.Wrap(err, "resolve space template apps")
			}
		}

		// create the space
		_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Create(ctx, space, metav1.CreateOptions{})
		if err != nil {
			return false, errors.Wrap(err, "create space")
		}

		// check if we should deploy apps
		if len(apps) > 0 {
			err = deploySpaceApps(clusterClient, space.Name, apps, cmd.Log)
			if err != nil {
				return false, err
			}
		}
		cmd.Log.Donef("Successfully created space %s in cluster %s", ansi.Color(space.Name, "white+b"), ansi.Color(cluster.Name, "white+b"))
	}
	
	return false, nil
}

type namespacedApp struct {
	App v1.App
	Namespace string
}

func resolveVClusterApps(managementClient kube.Interface, apps []storagev1.VirtualClusterAppReference) ([]namespacedApp, error) {
	appsList, err := managementClient.Loft().ManagementV1().Apps().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retApps := []namespacedApp{}
	for _, a := range apps {
		found := false
		for _, ma := range appsList.Items {
			if ma.Name == a.Name {
				namespace := "default"
				if a.Namespace != "" {
					namespace = a.Namespace
				}
				
				retApps = append(retApps, namespacedApp{
					App:       ma,
					Namespace: namespace,
				})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("couldn't find app %s. The app either doesn't exist or you have no access to use it", a)
		}
	}

	return retApps, nil
}
