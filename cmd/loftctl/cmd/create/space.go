package create

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/v2/pkg/client/naming"
	"os"
	"strconv"
	"time"

	clusterv1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/cluster/v1"
	agentstoragev1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/storage/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/cmd/use"
	"github.com/loft-sh/loftctl/v2/pkg/clihelper"
	"github.com/loft-sh/loftctl/v2/pkg/constants"
	"github.com/loft-sh/loftctl/v2/pkg/kube"
	"github.com/loft-sh/loftctl/v2/pkg/parameters"
	"github.com/loft-sh/loftctl/v2/pkg/task"
	"github.com/loft-sh/loftctl/v2/pkg/vcluster"
	"github.com/loft-sh/loftctl/v2/pkg/version"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	managementv1 "github.com/loft-sh/api/v2/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/v2/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/client/helper"
	"github.com/loft-sh/loftctl/v2/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	SleepAfter                   int64
	DeleteAfter                  int64
	Cluster                      string
	Project                      string
	CreateContext                bool
	SwitchContext                bool
	DisableDirectClusterEndpoint bool
	Template                     string
	Version                      string
	ParametersFile               string
	UseExisting                  bool

	DisplayName string
	Description string

	User string
	Team string

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
Creates a new space for the given project, if
it does not yet exist.

Example:
loft create space myspace
loft create space myspace --project myproject
loft create space myspace --project myproject --team myteam
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace create space ################
#######################################################
Creates a new space for the given project, if
it does not yet exist.

Example:
devspace create space myspace
devspace create space myspace --project myproject
devspace create space myspace --project myproject --team myteam
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

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.DisplayName, "display-name", "", "The display name to show in the UI for this virtual cluster")
	c.Flags().StringVar(&cmd.Description, "description", "", "The description to show in the UI for this virtual cluster")
	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVarP(&cmd.Project, "project", "p", "", "The project to use")
	c.Flags().StringVar(&cmd.User, "user", "", "The user to create the space for")
	c.Flags().StringVar(&cmd.Team, "team", "", "The team to create the space for")
	c.Flags().Int64Var(&cmd.SleepAfter, "sleep-after", 0, "DEPRECATED: If set to non zero, will tell the space to sleep after specified seconds of inactivity")
	c.Flags().Int64Var(&cmd.DeleteAfter, "delete-after", 0, "DEPRECATED: If set to non zero, will tell loft to delete the space after specified seconds of inactivity")
	c.Flags().BoolVar(&cmd.CreateContext, "create-context", true, "If loft should create a kube context for the space")
	c.Flags().BoolVar(&cmd.SwitchContext, "switch-context", true, "If loft should switch the current context to the new context")
	c.Flags().StringVar(&cmd.Template, "template", "", "The space template to use")
	c.Flags().StringVar(&cmd.Version, "version", "", "The template version to use")
	c.Flags().StringVar(&cmd.ParametersFile, "parameters", "", "The file where the parameter values for the apps are specified")
	c.Flags().BoolVar(&cmd.DisableDirectClusterEndpoint, "disable-direct-cluster-endpoint", false, "When enabled does not use an available direct cluster endpoint to connect to the space")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(args []string) error {
	spaceName := args[0]
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

	// create legacy space?
	if cmd.Project == "" {
		// create legacy space
		return cmd.legacyCreateSpace(baseClient, spaceName)
	}

	// create space
	return cmd.createSpace(baseClient, spaceName)
}

func (cmd *SpaceCmd) createSpace(baseClient client.Client, spaceName string) error {
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

	// determine space template to use
	spaceTemplate, err := helper.SelectSpaceTemplate(baseClient, cmd.Project, cmd.Template, cmd.Log)
	if err != nil {
		return err
	}

	// get parameters
	var templateParameters []storagev1.AppParameter
	if len(spaceTemplate.Spec.Versions) > 0 {
		if cmd.Version == "" {
			latestVersion := version.GetLatestVersion(spaceTemplate)
			if latestVersion == nil {
				return fmt.Errorf("couldn't find any version in template")
			}

			templateParameters = latestVersion.(*storagev1.SpaceTemplateVersion).Parameters
		} else {
			_, latestMatched, err := version.GetLatestMatchedVersion(spaceTemplate, cmd.Version)
			if err != nil {
				return err
			} else if latestMatched == nil {
				return fmt.Errorf("couldn't find any matching version to %s", cmd.Version)
			}

			templateParameters = latestMatched.(*storagev1.SpaceTemplateVersion).Parameters
		}
	} else {
		templateParameters = spaceTemplate.Spec.Parameters
	}

	// resolve space template parameters
	resolvedParameters, err := parameters.ResolveTemplateParameters(clihelper.GetDisplayName(spaceTemplate.Name, spaceTemplate.Spec.DisplayName), templateParameters, cmd.ParametersFile, cmd.Log)
	if err != nil {
		return err
	}

	// create space instance
	zone, offset := time.Now().Zone()
	spaceInstance := &managementv1.SpaceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: naming.ProjectNamespace(cmd.Project),
			Name:      spaceName,
			Annotations: map[string]string{
				clusterv1.SleepModeTimezoneAnnotation: zone + "#" + strconv.Itoa(offset),
			},
		},
		Spec: managementv1.SpaceInstanceSpec{
			SpaceInstanceSpec: storagev1.SpaceInstanceSpec{
				DisplayName: cmd.DisplayName,
				Description: cmd.Description,
				Owner: &storagev1.UserOrTeam{
					User: cmd.User,
					Team: cmd.Team,
				},
				TemplateRef: &storagev1.TemplateRef{
					Name:    spaceTemplate.Name,
					Version: cmd.Version,
				},
				ClusterRef: storagev1.ClusterRef{
					Cluster: cmd.Cluster,
				},
				Parameters: resolvedParameters,
			},
		},
	}

	// create space
	cmd.Log.Infof("Creating space %s in project %s...", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Project, "white+b"))
	spaceInstance, err = managementClient.Loft().ManagementV1().SpaceInstances(spaceInstance.Namespace).Create(context.TODO(), spaceInstance, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create virtual cluster")
	}

	// wait until space is ready
	spaceInstance, err = vcluster.WaitForSpaceInstance(context.TODO(), managementClient, spaceInstance.Namespace, spaceInstance.Name, cmd.Log)
	if err != nil {
		return err
	}
	cmd.Log.Donef("Successfully created the space %s in project %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Project, "white+b"))

	// should we create a kube context for the space
	if cmd.CreateContext {
		// create kube context options
		contextOptions, err := use.CreateSpaceInstanceOptions(baseClient, cmd.Config, cmd.Project, spaceInstance, cmd.DisableDirectClusterEndpoint, cmd.SwitchContext, cmd.Log)
		if err != nil {
			return err
		}

		// update kube config
		err = kubeconfig.UpdateKubeConfig(contextOptions)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use space %s in project %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Project, "white+b"))
	}

	return nil
}

func (cmd *SpaceCmd) legacyCreateSpace(baseClient client.Client, spaceName string) error {
	var err error

	// determine cluster name
	if cmd.Cluster == "" {
		cmd.Cluster, err = helper.SelectCluster(baseClient, cmd.Log)
		if err != nil {
			return err
		}
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

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get current user / team
	userName, teamName, err := helper.GetCurrentUser(context.TODO(), managementClient)
	if err != nil {
		return err
	}

	// create a cluster client
	clusterClient, err := baseClient.Cluster(cmd.Cluster)
	if err != nil {
		return err
	}

	// check if the cluster exists
	cluster, err := managementClient.Loft().ManagementV1().Clusters().Get(context.TODO(), cmd.Cluster, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			return fmt.Errorf("cluster '%s' does not exist, or you don't have permission to use it", cmd.Cluster)
		}

		return err
	}

	spaceNotFound := true
	if cmd.UseExisting {
		_, err := clusterClient.Agent().ClusterV1().Spaces().Get(context.TODO(), spaceName, metav1.GetOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return err
		}

		spaceNotFound = kerrors.IsNotFound(err)
	}

	if spaceNotFound {
		// get default space template
		spaceTemplate, err := resolveSpaceTemplate(managementClient, cluster, cmd.Template)
		if err != nil {
			return errors.Wrap(err, "resolve space template")
		} else if spaceTemplate != nil {
			cmd.Log.Infof("Using space template %s to create space %s", clihelper.GetDisplayName(spaceTemplate.Name, spaceTemplate.Spec.DisplayName), spaceName)
		}

		// create space object
		space := &clusterv1.Space{
			ObjectMeta: metav1.ObjectMeta{
				Name:        spaceName,
				Annotations: map[string]string{},
			},
		}
		if cmd.User != "" {
			space.Spec.User = cmd.User
		} else if cmd.Team != "" {
			space.Spec.Team = cmd.Team
		}
		if spaceTemplate != nil {
			space.Annotations = spaceTemplate.Spec.Template.Annotations
			space.Labels = spaceTemplate.Spec.Template.Labels
			if space.Annotations == nil {
				space.Annotations = map[string]string{}
			}
			space.Annotations["loft.sh/space-template"] = spaceTemplate.Name
			if spaceTemplate.Spec.Template.Objects != "" {
				space.Spec.Objects = spaceTemplate.Spec.Template.Objects
			}
		}
		if cmd.SleepAfter > 0 {
			space.Annotations[clusterv1.SleepModeSleepAfterAnnotation] = strconv.FormatInt(cmd.SleepAfter, 10)
		}
		if cmd.DeleteAfter > 0 {
			space.Annotations[clusterv1.SleepModeDeleteAfterAnnotation] = strconv.FormatInt(cmd.DeleteAfter, 10)
		}
		if cmd.DisplayName != "" {
			space.Annotations["loft.sh/display-name"] = cmd.DisplayName
		}
		if cmd.Description != "" {
			space.Annotations["loft.sh/description"] = cmd.Description
		}

		zone, offset := time.Now().Zone()
		space.Annotations[clusterv1.SleepModeTimezoneAnnotation] = zone + "#" + strconv.Itoa(offset)

		if spaceTemplate != nil && len(spaceTemplate.Spec.Template.Apps) > 0 {
			createTask := &managementv1.Task{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "create-space-",
				},
				Spec: managementv1.TaskSpec{
					TaskSpec: storagev1.TaskSpec{
						DisplayName: "Create Space " + spaceName,
						Target: storagev1.Target{
							Cluster: &storagev1.TargetCluster{
								Cluster: cmd.Cluster,
							},
						},
						Task: storagev1.TaskDefinition{
							SpaceCreationTask: &storagev1.SpaceCreationTask{
								Metadata: metav1.ObjectMeta{
									Name:        space.Name,
									Labels:      space.Labels,
									Annotations: space.Annotations,
								},
								Owner: &storagev1.UserOrTeam{
									User: space.Spec.User,
									Team: space.Spec.Team,
								},
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

			apps, err := resolveApps(managementClient, spaceTemplate.Spec.Template.Apps)
			if err != nil {
				return errors.Wrap(err, "resolve space template apps")
			}

			appsWithParameters, err := parameters.ResolveAppParameters(apps, cmd.ParametersFile, cmd.Log)
			if err != nil {
				return err
			}

			for _, appWithParameter := range appsWithParameters {
				createTask.Spec.Task.SpaceCreationTask.Apps = append(createTask.Spec.Task.SpaceCreationTask.Apps, agentstoragev1.AppReference{
					Name:       appWithParameter.App.Name,
					Parameters: appWithParameter.Parameters,
				})
			}

			// create the task and stream
			err = task.StreamTask(context.TODO(), managementClient, createTask, os.Stdout, cmd.Log)
			if err != nil {
				return err
			}
		} else {
			// create the space
			_, err = clusterClient.Agent().ClusterV1().Spaces().Create(context.TODO(), space, metav1.CreateOptions{})
			if err != nil {
				return errors.Wrap(err, "create space")
			}
		}

		cmd.Log.Donef("Successfully created the space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Cluster, "white+b"))
	}

	// should we create a kube context for the space
	if cmd.CreateContext || cmd.UseExisting {
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

		cmd.Log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(cmd.Cluster, "white+b"))
	}

	return nil
}

func resolveSpaceTemplate(client kube.Interface, cluster *managementv1.Cluster, template string) (*managementv1.SpaceTemplate, error) {
	if template == "" && cluster.Annotations != nil && cluster.Annotations[constants.LoftDefaultSpaceTemplate] != "" {
		template = cluster.Annotations[constants.LoftDefaultSpaceTemplate]
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

func resolveApps(client kube.Interface, apps []agentstoragev1.AppReference) ([]parameters.NamespacedApp, error) {
	appsList, err := client.Loft().ManagementV1().Apps().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retApps := []parameters.NamespacedApp{}
	for _, a := range apps {
		found := false
		for _, ma := range appsList.Items {
			if a.Name == "" {
				continue
			}
			if ma.Name == a.Name {
				app := ma
				retApps = append(retApps, parameters.NamespacedApp{
					App: &app,
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
