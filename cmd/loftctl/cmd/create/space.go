package create

import (
	"context"
	"fmt"
	clusterv1 "github.com/loft-sh/agentapi/pkg/apis/loft/cluster/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/cmd/use"
	"github.com/loft-sh/loftctl/pkg/kube"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"os"
	"os/signal"
	"strconv"

	"github.com/loft-sh/agentapi/pkg/apis/kiosk/config/v1alpha1"
	tenancyv1alpha1 "github.com/loft-sh/agentapi/pkg/apis/kiosk/tenancy/v1alpha1"
	v1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LoftHelmReleaseAppLabel indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppLabel = "loft.sh/app"
	
	// LoftHelmReleaseAppNameLabel indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppNameLabel = "loft.sh/app-name"

	// LoftHelmReleaseAppResourceVersionAnnotation indicates the resource version of the loft app
	LoftHelmReleaseAppResourceVersionAnnotation = "loft.sh/app-resource-version"
	
	// LoftDefaultSpaceTemplate indicates the default space template on a cluster
	LoftDefaultSpaceTemplate = "space.loft.sh/default-template"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	SleepAfter                   int64
	DeleteAfter                  int64
	Cluster                      string
	Account                      string
	CreateContext                bool
	SwitchContext                bool
	DisableDirectClusterEndpoint bool
	Template                     string

	Log log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
################## loft create space ##################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
loft create space myspace
loft create space myspace --cluster mycluster
loft create space myspace --cluster mycluster --account myaccount
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace create space ################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
devspace create space myspace
devspace create space myspace --cluster mycluster
devspace create space myspace --cluster mycluster --account myaccount
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "space",
		Short: "Creates a new space in the given cluster",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVar(&cmd.Account, "account", "", "The cluster account to use")
	c.Flags().Int64Var(&cmd.SleepAfter, "sleep-after", 0, "If set to non zero, will tell the space to sleep after specified seconds of inactivity")
	c.Flags().Int64Var(&cmd.DeleteAfter, "delete-after", 0, "If set to non zero, will tell loft to delete the space after specified seconds of inactivity")
	c.Flags().BoolVar(&cmd.CreateContext, "create-context", true, "If loft should create a kube context for the space")
	c.Flags().BoolVar(&cmd.SwitchContext, "switch-context", true, "If loft should switch the current context to the new context")
	c.Flags().StringVar(&cmd.Template, "template", "", "The space template to use")
	c.Flags().BoolVar(&cmd.DisableDirectClusterEndpoint, "disable-direct-cluster-endpoint", false, "When enabled does not use an available direct cluster endpoint to connect to the space")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(cobraCmd *cobra.Command, args []string) error {
	spaceName := args[0]
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// determine cluster name
	clusterName := cmd.Cluster
	if clusterName == "" {
		clusterName, err = helper.SelectCluster(baseClient, cmd.Log)
		if err != nil {
			return err
		}
	}

	// determine account name
	accountName := cmd.Account
	if accountName == "" {
		accountName, err = helper.SelectAccount(baseClient, clusterName, cmd.Log)
		if err != nil {
			return err
		}
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get owner references
	ownerReferences, err := getOwnerReferences(managementClient, clusterName, accountName)
	if err != nil {
		return err
	}

	// create a cluster client
	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	// check if the cluster exists
	cluster, err := managementClient.Loft().ManagementV1().Clusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			return fmt.Errorf("cluster '%s' does not exist, or you don't have permission to use it", clusterName)
		}

		return err
	}
	
	// get default space template
	spaceTemplate, err := resolveSpaceTemplate(managementClient, cluster, cmd.Template)
	if err != nil {
		return errors.Wrap(err, "resolve space template")
	} else if spaceTemplate != nil {
		cmd.Log.Infof("Using space template %s to create space %s", spaceTemplate.Name, spaceName)
	}
	
	var apps []v1.App
	if spaceTemplate != nil && len(spaceTemplate.Spec.Template.Apps) > 0 {
		apps, err = resolveApps(managementClient, spaceTemplate.Spec.Template.Apps)
		if err != nil {
			return errors.Wrap(err, "resolve space template apps")
		}
	}

	// create space object
	space := &tenancyv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:            spaceName,
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

	// create the space
	_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Create(context.TODO(), space, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create space")
	}

	// cleanup on ctrl+c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func(){
		<-c
		_ = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Delete(context.TODO(), spaceName, metav1.DeleteOptions{})
		os.Exit(1)
	}()
	
	// check if we should deploy apps
	if len(apps) > 0 {
		err = deploySpaceApps(clusterClient, space.Name, apps, cmd.Log)
		if err != nil {
			return err
		}
	}

	cmd.Log.Donef("Successfully created the space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// should we create a kube context for the space
	if cmd.CreateContext {
		// create kube context options
		contextOptions, err := use.CreateClusterContextOptions(baseClient, cmd.Config, cluster, spaceName, cmd.DisableDirectClusterEndpoint, cmd.SwitchContext, cmd.Log)
		if err != nil {
			return err
		}

		// update kube config
		err = kubeconfig.UpdateKubeConfig(contextOptions)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
	}

	return nil
}

func deploySpaceApps(clusterClient kube.Interface, space string, apps []v1.App, log log.Logger) error {
	log.Infof("Creating space %s...", space)
	for _, app := range apps {
		log.Infof("Deploying app %s...", app.Name)
		_, err := clusterClient.Agent().ClusterV1().HelmReleases(space).Create(context.TODO(), convertAppToHelmRelease(app), metav1.CreateOptions{})
		if err != nil {
			_ = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Delete(context.TODO(), space, metav1.DeleteOptions{})
			return errors.Wrap(err, "deploy app " + app.Name)
		}
	}
	
	return nil
}

func getOwnerReferences(client kube.Interface, cluster, accountName string) ([]metav1.OwnerReference, error) {
	clusterMembers, err := client.Loft().ManagementV1().Clusters().ListMembers(context.TODO(), cluster, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	account := findOwnerAccount(append(clusterMembers.Teams, clusterMembers.Users...), accountName)

	if account != nil {
		t := true
		return []metav1.OwnerReference{
			{
				APIVersion: account.APIVersion,
				Kind:       account.Kind,
				Name:       account.Name,
				UID:        account.UID,
				Controller: &t, 
			},
		}, nil
	}

	return []metav1.OwnerReference{}, nil
}

func findOwnerAccount(haystack []v1.ClusterMember, needle string) *v1alpha1.Account {
	for _, member := range haystack {
		if member.Account != nil && member.Account.Name == needle {
			return member.Account
		}
	}
	return nil
}

func convertAppToHelmRelease(app v1.App) *clusterv1.HelmRelease {
	release := &clusterv1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name: app.Name,
			Labels: map[string]string{
				LoftHelmReleaseAppLabel: "true",
				LoftHelmReleaseAppNameLabel: app.Name,
			},
			Annotations: map[string]string{
				LoftHelmReleaseAppResourceVersionAnnotation: app.ResourceVersion,
			},
		},
		Spec: clusterv1.HelmReleaseSpec{
			Manifests: app.Spec.Manifests,
		},
	}
	if app.Spec.Helm != nil {
		release.Spec.Chart = clusterv1.Chart{
			Name:     app.Spec.Helm.Name,
			Version:  app.Spec.Helm.Version,
			RepoURL:  app.Spec.Helm.RepoURL,
			Username: app.Spec.Helm.Username,
			Password: string(app.Spec.Helm.Password),
		}
		release.Spec.Config = app.Spec.Helm.Values
		release.Spec.InsecureSkipTlsVerify = app.Spec.Helm.Insecure
	}
	return release
}

func resolveSpaceTemplate(client kube.Interface, cluster *v1.Cluster, template string) (*v1.SpaceTemplate, error) {
	if template == "" && cluster.Annotations != nil && cluster.Annotations[LoftDefaultSpaceTemplate] != "" {
		template = cluster.Annotations[LoftDefaultSpaceTemplate]
	}
	
	if template != "" {
		spaceTemplate, err := client.Loft().ManagementV1().SpaceTemplates().Get(context.TODO(), template, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		
		return spaceTemplate, nil
	}
	
	return nil, nil
}

func resolveApps(client kube.Interface, apps []storagev1.SpaceAppReference) ([]v1.App, error) {
	appsList, err := client.Loft().ManagementV1().Apps().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	retApps := []v1.App{}
	for _, a := range apps {
		found := false
		for _, ma := range appsList.Items {
			if a.Name == "" {
				continue
			}
			if ma.Name == a.Name {
				retApps = append(retApps, ma)
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


