package devpod

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v3/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd/devpod/list"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/naming"
	devpodpkg "github.com/loft-sh/loftctl/v3/pkg/devpod"
	"github.com/loft-sh/loftctl/v3/pkg/kube"
	"github.com/loft-sh/loftctl/v3/pkg/parameters"
	"github.com/loft-sh/loftctl/v3/pkg/remotecommand"
	"github.com/loft-sh/log"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// UpCmd holds the cmd flags:
type UpCmd struct {
	*flags.GlobalFlags

	Log log.Logger
}

// NewUpCmd creates a new command
func NewUpCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &UpCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Hidden: true,
		Use:    "up",
		Short:  "Runs up on a workspace",
		Long: `
#######################################################
#################### loft devpod up ###################
#######################################################
	`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context(), os.Stdin, os.Stdout, os.Stderr)
		},
	}

	return c
}

func (cmd *UpCmd) Run(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	info, err := devpodpkg.GetWorkspaceInfoFromEnv()
	if err != nil {
		return err
	}
	workspace, err := devpodpkg.FindWorkspace(ctx, baseClient, info.UID, info.ProjectName)
	if err != nil {
		return err
	}

	// create workspace if doesn't exist
	if workspace == nil {
		workspace, err = createWorkspace(ctx, baseClient, cmd.Log.ErrorStreamOnly())
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
	}

	conn, err := devpodpkg.DialWorkspace(baseClient, workspace, "up", devpodpkg.OptionsFromEnv(storagev1.DevPodFlagsUp))
	if err != nil {
		return err
	}

	_, err = remotecommand.ExecuteConn(ctx, conn, stdin, stdout, stderr, cmd.Log.ErrorStreamOnly())
	if err != nil {
		return fmt.Errorf("error executing: %w", err)
	}

	return nil
}

func createWorkspace(ctx context.Context, baseClient client.Client, log log.Logger) (*managementv1.DevPodWorkspaceInstance, error) {
	workspaceInfo, err := devpodpkg.GetWorkspaceInfoFromEnv()
	if err != nil {
		return nil, err
	}

	// get template
	template := os.Getenv(devpodpkg.LoftTemplateOption)
	if template == "" {
		return nil, fmt.Errorf("%s is missing in environment", devpodpkg.LoftTemplateOption)
	}

	// create client
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, err
	}

	// get template version
	templateVersion := os.Getenv(devpodpkg.LoftTemplateVersionOption)
	if templateVersion == "latest" {
		templateVersion = ""
	}

	// find parameters
	resolvedParameters, err := getParametersFromEnvironment(ctx, managementClient, workspaceInfo.ProjectName, template, templateVersion)
	if err != nil {
		return nil, fmt.Errorf("resolve parameters: %w", err)
	}

	// get workspace picture
	workspacePicture := os.Getenv("WORKSPACE_PICTURE")
	// get workspace source
	workspaceSource := os.Getenv("WORKSPACE_SOURCE")

	workspace := &managementv1.DevPodWorkspaceInstance{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: naming.SafeConcatNameMax([]string{workspaceInfo.ID}, 53) + "-",
			Namespace:    naming.ProjectNamespace(workspaceInfo.ProjectName),
			Labels: map[string]string{
				storagev1.DevPodWorkspaceIDLabel:  workspaceInfo.ID,
				storagev1.DevPodWorkspaceUIDLabel: workspaceInfo.UID,
			},
			Annotations: map[string]string{
				storagev1.DevPodWorkspacePictureAnnotation: workspacePicture,
				storagev1.DevPodWorkspaceSourceAnnotation:  workspaceSource,
			},
		},
		Spec: managementv1.DevPodWorkspaceInstanceSpec{
			DevPodWorkspaceInstanceSpec: storagev1.DevPodWorkspaceInstanceSpec{
				DisplayName: workspaceInfo.ID,
				Parameters:  resolvedParameters,
				TemplateRef: &storagev1.TemplateRef{
					Name:    template,
					Version: templateVersion,
				},
			},
		},
	}

	// check if runner is defined
	runnerName := os.Getenv("LOFT_RUNNER")
	if runnerName != "" {
		workspace.Spec.RunnerRef.Runner = runnerName
	}

	// create instance
	workspace, err = managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(naming.ProjectNamespace(workspaceInfo.ProjectName)).Create(ctx, workspace, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	log.Infof("Created workspace %s", workspace.Name)

	// we need to wait until instance is scheduled
	err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (done bool, err error) {
		workspace, err = managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(naming.ProjectNamespace(workspaceInfo.ProjectName)).Get(ctx, workspace.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if !isReady(workspace) {
			log.Debugf("Workspace %s is in phase %s, waiting until its ready", workspace.Name, workspace.Status.Phase)
			return false, nil
		}

		log.Debugf("Workspace %s is ready", workspace.Name)
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait for instance to get ready: %w", err)
	}

	return workspace, nil
}

func getParametersFromEnvironment(ctx context.Context, kubeClient kube.Interface, projectName, templateName, templateVersion string) (string, error) {
	// are there any parameters in environment?
	environmentVariables := os.Environ()
	envMap := map[string]string{}
	for _, v := range environmentVariables {
		splitted := strings.SplitN(v, "=", 2)
		if len(splitted) != 2 {
			continue
		} else if !strings.HasPrefix(splitted[0], "TEMPLATE_OPTION_") {
			continue
		}

		envMap[splitted[0]] = splitted[1]
	}
	if len(envMap) == 0 {
		return "", nil
	}

	// find these in the template
	template, err := list.FindTemplate(ctx, kubeClient, projectName, templateName)
	if err != nil {
		return "", fmt.Errorf("find template: %w", err)
	}

	// find version
	var templateParameters []storagev1.AppParameter
	if len(template.Spec.Versions) > 0 {
		templateParameters, err = list.GetTemplateParameters(template, templateVersion)
		if err != nil {
			return "", err
		}
	} else {
		templateParameters = template.Spec.Parameters
	}

	// parse versions
	outMap := map[string]interface{}{}
	for _, parameter := range templateParameters {
		// check if its in environment
		val := envMap[list.VariableToEnvironmentVariable(parameter.Variable)]
		outVal, err := parameters.VerifyValue(val, parameter)
		if err != nil {
			return "", fmt.Errorf("validate parameter %s: %w", parameter.Variable, err)
		}

		outMap[parameter.Variable] = outVal
	}

	// convert to string
	out, err := yaml.Marshal(outMap)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isReady(workspace *managementv1.DevPodWorkspaceInstance) bool {
	return workspace.Status.Phase == storagev1.InstanceReady
}
