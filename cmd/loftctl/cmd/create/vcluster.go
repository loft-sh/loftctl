package create

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/v2/pkg/client/naming"
	"github.com/loft-sh/loftctl/v2/pkg/vcluster"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	clusterv1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/cluster/v1"
	agentstoragev1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/storage/v1"
	managementv1 "github.com/loft-sh/api/v2/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/cmd/use"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/clihelper"
	"github.com/loft-sh/loftctl/v2/pkg/constants"
	"github.com/loft-sh/loftctl/v2/pkg/kube"
	"github.com/loft-sh/loftctl/v2/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/parameters"
	"github.com/loft-sh/loftctl/v2/pkg/random"
	"github.com/loft-sh/loftctl/v2/pkg/task"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/loft-sh/loftctl/v2/pkg/version"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualClusterCmd holds the cmd flags
type VirtualClusterCmd struct {
	*flags.GlobalFlags

	SleepAfter     int64
	DeleteAfter    int64
	Image          string
	Cluster        string
	Space          string
	Template       string
	Project        string
	CreateContext  bool
	SwitchContext  bool
	Print          bool
	SkipWait       bool
	UseExisting    bool
	ParametersFile string
	Version        string

	DisplayName string
	Description string

	User string
	Team string

	DisableDirectClusterEndpoint bool
	AccessPointCertificateTTL    int32

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
loft create vcluster test --project myproject
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
devspace create vcluster test --project myproject
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

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.DisplayName, "display-name", "", "The display name to show in the UI for this virtual cluster")
	c.Flags().StringVar(&cmd.Description, "description", "", "The description to show in the UI for this virtual cluster")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to create the virtual cluster in")
	c.Flags().StringVarP(&cmd.Project, "project", "p", "", "The project to use")
	c.Flags().StringVar(&cmd.Space, "space", "", "The space to create the virtual cluster in")
	c.Flags().StringVar(&cmd.User, "user", "", "The user to create the space for")
	c.Flags().StringVar(&cmd.Team, "team", "", "The team to create the space for")
	c.Flags().BoolVar(&cmd.Print, "print", false, "If enabled, prints the context to the console")
	c.Flags().Int64Var(&cmd.SleepAfter, "sleep-after", 0, "DEPRECATED: If set to non zero, will tell the space to sleep after specified seconds of inactivity")
	c.Flags().Int64Var(&cmd.DeleteAfter, "delete-after", 0, "DEPRECATED: If set to non zero, will tell loft to delete the space after specified seconds of inactivity")
	c.Flags().BoolVar(&cmd.CreateContext, "create-context", true, "If loft should create a kube context for the space")
	c.Flags().BoolVar(&cmd.SwitchContext, "switch-context", true, "If loft should switch the current context to the new context")
	c.Flags().BoolVar(&cmd.SkipWait, "skip-wait", false, "If true, will not wait until the virtual cluster is running")
	c.Flags().BoolVar(&cmd.UseExisting, "use", false, "If loft should use the virtual cluster if its already there")
	c.Flags().StringVar(&cmd.Template, "template", "", "The virtual cluster template to use to create the virtual cluster")
	c.Flags().StringVar(&cmd.Version, "version", "", "The template version to use")
	c.Flags().StringVar(&cmd.ParametersFile, "parameters", "", "The file where the parameter values for the apps are specified")
	c.Flags().BoolVar(&cmd.DisableDirectClusterEndpoint, "disable-direct-cluster-endpoint", false, "When enabled does not use an available direct cluster endpoint to connect to the vcluster")
	c.Flags().Int32Var(&cmd.AccessPointCertificateTTL, "ttl", 86_400, "Sets certificate TTL when using virtual cluster via access point")
	return c
}

// Run executes the command
func (cmd *VirtualClusterCmd) Run(args []string) error {
	virtualClusterName := args[0]
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	err = client.VerifyVersion(baseClient)
	if err != nil {
		return err
	}

	// determine cluster name
	cmd.Cluster, cmd.Project, err = helper.SelectProjectOrCluster(baseClient, cmd.Cluster, cmd.Project, cmd.Log)
	if err != nil {
		return err
	}

	// create legacy virtual cluster?
	if cmd.Project == "" {
		// create legacy virtual cluster
		return cmd.legacyCreateVirtualCluster(baseClient, virtualClusterName)
	}

	// create project virtual cluster
	return cmd.createVirtualCluster(baseClient, virtualClusterName)
}

func (cmd *VirtualClusterCmd) createVirtualCluster(baseClient client.Client, virtualClusterName string) error {
	virtualClusterNamespace := naming.ProjectNamespace(cmd.Project)
	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get current user / team
	if cmd.User == "" && cmd.Team == "" {
		userName, teamName, err := helper.GetCurrentUser(context.TODO(), managementClient)
		if err != nil {
			return err
		}
		if userName != nil {
			cmd.User = userName.Name
		} else {
			cmd.Team = teamName.Name
		}
	}

	var virtualClusterInstance *managementv1.VirtualClusterInstance
	if cmd.UseExisting {
		virtualClusterInstance, err = managementClient.Loft().ManagementV1().VirtualClusterInstances(virtualClusterNamespace).Get(context.TODO(), virtualClusterName, metav1.GetOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			cmd.Log.Debugf("Couldn't retrieve space instance: %v", err)
		}
	}

	// create virtual cluster if necessary
	if virtualClusterInstance == nil {

		// determine space template to use
		virtualClusterTemplate, err := helper.SelectVirtualClusterTemplate(baseClient, cmd.Project, cmd.Template, cmd.Log)
		if err != nil {
			return err
		}

		// get parameters
		var templateParameters []storagev1.AppParameter
		if len(virtualClusterTemplate.Spec.Versions) > 0 {
			if cmd.Version == "" {
				latestVersion := version.GetLatestVersion(virtualClusterTemplate)
				if latestVersion == nil {
					return fmt.Errorf("couldn't find any version in template")
				}

				templateParameters = latestVersion.(*storagev1.SpaceTemplateVersion).Parameters
			} else {
				_, latestMatched, err := version.GetLatestMatchedVersion(virtualClusterTemplate, cmd.Version)
				if err != nil {
					return err
				} else if latestMatched == nil {
					return fmt.Errorf("couldn't find any matching version to %s", cmd.Version)
				}

				templateParameters = latestMatched.(*storagev1.SpaceTemplateVersion).Parameters
			}
		} else {
			templateParameters = virtualClusterTemplate.Spec.Parameters
		}

		// resolve space template parameters
		resolvedParameters, err := parameters.ResolveTemplateParameters(clihelper.GetDisplayName(virtualClusterTemplate.Name, virtualClusterTemplate.Spec.DisplayName), templateParameters, cmd.ParametersFile, cmd.Log)
		if err != nil {
			return err
		}

		// create space instance
		zone, offset := time.Now().Zone()
		virtualClusterInstance = &managementv1.VirtualClusterInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: naming.ProjectNamespace(cmd.Project),
				Name:      virtualClusterName,
				Annotations: map[string]string{
					clusterv1.SleepModeTimezoneAnnotation: zone + "#" + strconv.Itoa(offset),
				},
			},
			Spec: managementv1.VirtualClusterInstanceSpec{
				VirtualClusterInstanceSpec: storagev1.VirtualClusterInstanceSpec{
					DisplayName: cmd.DisplayName,
					Description: cmd.Description,
					Owner: &storagev1.UserOrTeam{
						User: cmd.User,
						Team: cmd.Team,
					},
					TemplateRef: &storagev1.TemplateRef{
						Name: virtualClusterTemplate.Name,
					},
					ClusterRef: storagev1.VirtualClusterClusterRef{
						ClusterRef: storagev1.ClusterRef{Cluster: cmd.Cluster},
					},
					Parameters: resolvedParameters,
				},
			},
		}
		// create space
		cmd.Log.Infof("Creating virtual cluster %s in project %s...", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Project, "white+b"))
		virtualClusterInstance, err = managementClient.Loft().ManagementV1().VirtualClusterInstances(virtualClusterInstance.Namespace).Create(context.TODO(), virtualClusterInstance, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "create virtual cluster")
		}
	}

	// wait until virtual cluster is ready
	virtualClusterInstance, err = vcluster.WaitForVirtualClusterInstance(context.TODO(), managementClient, virtualClusterInstance.Namespace, virtualClusterInstance.Name, !cmd.SkipWait, cmd.Log)
	if err != nil {
		return err
	}
	cmd.Log.Donef("Successfully created the virtual cluster %s in project %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Project, "white+b"))

	// should we create a kube context for the space
	if cmd.CreateContext {
		// create kube context options
		contextOptions, err := use.CreateVirtualClusterInstanceOptions(baseClient, cmd.Config, cmd.Project, virtualClusterInstance, cmd.DisableDirectClusterEndpoint, cmd.SwitchContext, cmd.Log)
		if err != nil {
			return err
		}

		// update kube config
		err = kubeconfig.UpdateKubeConfig(contextOptions)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use virtual cluster %s in project %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Project, "white+b"))
	}

	return nil
}

func (cmd *VirtualClusterCmd) legacyCreateVirtualCluster(baseClient client.Client, virtualClusterName string) error {
	if cmd.UseExisting {
		cmd.Log.Warnf("--use is not supported for legacy virtual cluster creation, please specify a project instead")
	}
	if cmd.SkipWait {
		cmd.Log.Warnf("--skip-wait is not supported for legacy virtual cluster creation, please specify a project instead")
	}

	ctx := context.Background()

	// determine space name
	if cmd.Space == "" {
		cmd.Space = "vcluster-" + virtualClusterName + "-" + random.RandomString(5)
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

	// get current user / team
	userName, teamName, err := helper.GetCurrentUser(context.TODO(), managementClient)
	if err != nil {
		return err
	}

	var (
		vClusterChartName           string
		vClusterValues              string
		vClusterVersion             string
		vClusterTemplate            *storagev1.VirtualClusterTemplateSpec
		vClusterTemplateName        string
		vClusterTemplateDisplayName string
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
			vClusterTemplateDisplayName = clihelper.GetDisplayName(defaults.DefaultTemplate.Name, defaults.DefaultTemplate.Spec.DisplayName)
		}
	} else {
		virtualClusterTemplate, err := managementClient.Loft().ManagementV1().VirtualClusterTemplates().Get(ctx, cmd.Template, metav1.GetOptions{})
		if err != nil {
			return err
		}
		vClusterChartName = virtualClusterTemplate.Spec.Template.HelmRelease.Chart.Name
		vClusterValues = virtualClusterTemplate.Spec.Template.HelmRelease.Values
		vClusterVersion = virtualClusterTemplate.Spec.Template.HelmRelease.Chart.Version
		vClusterTemplate = &virtualClusterTemplate.Spec.VirtualClusterTemplateSpec
		vClusterTemplateName = virtualClusterTemplate.Name
		vClusterTemplateDisplayName = clihelper.GetDisplayName(virtualClusterTemplate.Name, virtualClusterTemplate.Spec.DisplayName)
	}

	// create the task
	createTask := &managementv1.Task{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "create-vcluster-",
		},
		Spec: managementv1.TaskSpec{
			TaskSpec: storagev1.TaskSpec{
				DisplayName: "Create Virtual Cluster " + virtualClusterName,
				Target: storagev1.Target{
					Cluster: &storagev1.TargetCluster{
						Cluster: cmd.Cluster,
					},
				},
				Task: storagev1.TaskDefinition{
					VirtualClusterCreationTask: &storagev1.VirtualClusterCreationTask{
						Metadata: metav1.ObjectMeta{
							Name:      virtualClusterName,
							Namespace: cmd.Space,
						},
						HelmRelease: agentstoragev1.VirtualClusterHelmRelease{
							Chart: agentstoragev1.VirtualClusterHelmChart{
								Name:    vClusterChartName,
								Version: vClusterVersion,
							},
							Values: vClusterValues,
						},
						Wait:              true,
						Apps:              nil,
						SpaceCreationTask: nil,
					},
				},
			},
		},
	}
	if userName != nil {
		createTask.Spec.Access = []storagev1.Access{
			{
				Verbs:        []string{"*"},
				Subresources: []string{"*"},
				Users:        []string{userName.Name},
			},
		}
	} else if teamName != nil {
		createTask.Spec.Access = []storagev1.Access{
			{
				Verbs:        []string{"*"},
				Subresources: []string{"*"},
				Teams:        []string{teamName.Name},
			},
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
	err = cmd.createSpace(ctx, baseClient, clusterClient, managementClient, vClusterTemplate, cluster, createTask)
	if err != nil {
		return errors.Wrap(err, "create space")
	}

	// create the object
	if vClusterTemplate != nil {
		cmd.Log.Infof("Using virtual cluster template %s", vClusterTemplateDisplayName)
		createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations = vClusterTemplate.Template.Annotations
		createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Labels = vClusterTemplate.Template.Labels
		if createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations == nil {
			createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations = map[string]string{}
		}
		createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations["loft.sh/virtual-cluster-template"] = vClusterTemplateName
		createTask.Spec.Task.VirtualClusterCreationTask.Access = vClusterTemplate.Template.Access
	}

	if cmd.DisplayName != "" {
		if createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations == nil {
			createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations = map[string]string{}
		}
		createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations["loft.sh/display-name"] = cmd.DisplayName
	}
	if cmd.Description != "" {
		if createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations == nil {
			createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations = map[string]string{}
		}
		createTask.Spec.Task.VirtualClusterCreationTask.Metadata.Annotations["loft.sh/description"] = cmd.Description
	}

	// resolve apps
	if vClusterTemplate != nil && len(vClusterTemplate.Template.Apps) > 0 {
		vClusterApps, err := resolveVClusterApps(managementClient, vClusterTemplate.Template.Apps)
		if err != nil {
			return errors.Wrap(err, "resolve virtual cluster template apps")
		}

		appsWithParameters, err := parameters.ResolveAppParameters(vClusterApps, cmd.ParametersFile, cmd.Log)
		if err != nil {
			return err
		}

		for _, appWithParameter := range appsWithParameters {
			createTask.Spec.Task.VirtualClusterCreationTask.Apps = append(createTask.Spec.Task.VirtualClusterCreationTask.Apps, agentstoragev1.AppReference{
				Name:       appWithParameter.App.Name,
				Namespace:  appWithParameter.Namespace,
				Parameters: appWithParameter.Parameters,
			})
		}
	}

	// create the task and stream
	err = task.StreamTask(context.TODO(), managementClient, createTask, os.Stdout, cmd.Log)
	if err != nil {
		return err
	}

	cmd.Log.Donef("Successfully created the virtual cluster %s in cluster %s and space %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(cmd.Cluster, "white+b"), ansi.Color(cmd.Space, "white+b"))

	// should we create a kube context for the virtual context
	if cmd.CreateContext || cmd.Print {
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

func (cmd *VirtualClusterCmd) createSpace(ctx context.Context, baseClient client.Client, clusterClient kube.Interface, managementClient kube.Interface, vClusterTemplate *storagev1.VirtualClusterTemplateSpec, cluster *managementv1.Cluster, task *managementv1.Task) error {
	_, err := clusterClient.Agent().ClusterV1().Spaces().Get(ctx, cmd.Space, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) == false {
			return err
		}

		// determine user or team name
		if cmd.User == "" && cmd.Team == "" {
			user, team, err := helper.SelectUserOrTeam(baseClient, cmd.Cluster, cmd.Log)
			if err != nil {
				return err
			} else if user != nil {
				cmd.User = user.Name
			} else if team != nil {
				cmd.Team = team.Name
			}
		}

		// resolve space template
		template := ""
		if vClusterTemplate != nil && vClusterTemplate.SpaceTemplateRef != nil {
			template = vClusterTemplate.SpaceTemplateRef.Name
		}

		// get space template
		spaceTemplate, err := resolveSpaceTemplate(managementClient, cluster, template)
		if err != nil {
			return errors.Wrap(err, "resolve space template")
		} else if spaceTemplate != nil {
			cmd.Log.Infof("Using space template %s to create space %s", clihelper.GetDisplayName(spaceTemplate.Name, spaceTemplate.Spec.DisplayName), cmd.Space)
		}

		// add to task
		task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask = &storagev1.SpaceCreationTask{
			Metadata: metav1.ObjectMeta{
				Name:        cmd.Space,
				Annotations: map[string]string{},
			},
			Owner: &storagev1.UserOrTeam{
				User: cmd.User,
				Team: cmd.Team,
			},
			Apps: nil,
		}
		if spaceTemplate != nil {
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations = spaceTemplate.Spec.Template.Annotations
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Labels = spaceTemplate.Spec.Template.Labels
			if task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations == nil {
				task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations = map[string]string{}
			}
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations["loft.sh/space-template"] = spaceTemplate.Name
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Objects = spaceTemplate.Spec.Template.Objects
		}
		if cmd.SleepAfter > 0 {
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations[clusterv1.SleepModeSleepAfterAnnotation] = strconv.FormatInt(cmd.SleepAfter, 10)
		}
		if cmd.DeleteAfter > 0 {
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations[clusterv1.SleepModeDeleteAfterAnnotation] = strconv.FormatInt(cmd.DeleteAfter, 10)
		}
		zone, offset := time.Now().Zone()
		task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations[clusterv1.SleepModeTimezoneAnnotation] = zone + "#" + strconv.Itoa(offset)
		if task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations["loft.sh/description"] == "" {
			task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations["loft.sh/description"] = "Space for Virtual Cluster [" + task.Spec.Task.VirtualClusterCreationTask.Metadata.Name + "](/vclusters#search=" + task.Spec.Task.VirtualClusterCreationTask.Metadata.Name + ")"
		}
		task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Metadata.Annotations[constants.VClusterSpace] = "true"

		// resolve the space apps
		if spaceTemplate != nil && len(spaceTemplate.Spec.Template.Apps) > 0 {
			apps, err := resolveApps(managementClient, spaceTemplate.Spec.Template.Apps)
			if err != nil {
				return errors.Wrap(err, "resolve space template apps")
			}

			for _, appWithoutParameters := range apps {
				task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Apps = append(task.Spec.Task.VirtualClusterCreationTask.SpaceCreationTask.Apps, agentstoragev1.AppReference{
					Name: appWithoutParameters.App.Name,
				})
			}
		}
	}

	return nil
}

func resolveVClusterApps(managementClient kube.Interface, apps []agentstoragev1.AppReference) ([]parameters.NamespacedApp, error) {
	appsList, err := managementClient.Loft().ManagementV1().Apps().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retApps := []parameters.NamespacedApp{}
	for _, a := range apps {
		found := false
		for _, ma := range appsList.Items {
			if ma.Name == a.Name {
				namespace := "default"
				if a.Namespace != "" {
					namespace = a.Namespace
				}

				m := ma
				retApps = append(retApps, parameters.NamespacedApp{
					App:       &m,
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
