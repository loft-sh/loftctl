package devpod

import (
	"context"
	"fmt"
	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/v3/pkg/kube"
	"os"
	"path/filepath"

	storagev1 "github.com/loft-sh/api/v3/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/naming"
	"github.com/loft-sh/log"
	"github.com/spf13/cobra"
	"gopkg.in/square/go-jose.v2/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/devpod/pkg/provider"
	"github.com/loft-sh/devpod/pkg/types"
)

// ImportCmd holds the cmd flags
type ImportCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewImportCmd creates a new command
func NewImportCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ImportCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:   "import",
		Short: "Imports a workspace",
		Long: `
#######################################################
################# loft devpod import ##################
#######################################################
	`,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Import(cobraCmd.Context(), args)
		},
	}

	return c
}

func (cmd *ImportCmd) Import(ctx context.Context, args []string) error {
	id := os.Getenv("WORKSPACE_ID")
	if id == "" {
		return fmt.Errorf("%s is missing in environment", "WORKSPACE_ID")
	}

	providerID := os.Getenv("PROVIDER_ID")
	if id == "" {
		return fmt.Errorf("%s is missing in environment", "WORKSPACE_ID")
	}

	workspaceUID := os.Getenv("WORKSPACE_UID")
	if workspaceUID == "" {
		return fmt.Errorf("%s is missing in environment", "WORKSPACE_UID")
	}

	workspaceFolder := os.Getenv("WORKSPACE_FOLDER")
	if workspaceUID == "" {
		return fmt.Errorf("%s is missing in environment", "WORKSPACE_FOLDER")
	}

	workspaceContext := os.Getenv("WORKSPACE_CONTEXT")
	if workspaceUID == "" {
		return fmt.Errorf("%s is missing in environment", "WORKSPACE_CONTEXT")
	}

	projectName := os.Getenv("LOFT_PROJECT")
	if projectName == "" {
		return fmt.Errorf("%s is missing in environment", "LOFT_PROJECT")
	}

	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// create client
	managementClient, err := baseClient.Management()
	if err != nil {
		return fmt.Errorf("create management client: %w", err)
	}

	workspace, err := cmd.fetchWorkspace(ctx, managementClient, workspaceUID, projectName)
	if err != nil {
		return err
	}

	providerObj := provider.WorkspaceProviderConfig{
		Name: providerID,
		Options: map[string]config.OptionValue{
			"LOFT_PROJECT":  {Value: projectName},
			"LOFT_TEMPLATE": {Value: workspace.Spec.DevPodWorkspaceInstanceSpec.TemplateRef.Name},
		},
	}

	providerWorkspaceSource, err := provider.ParseWorkspaceSource(workspace.Annotations[storagev1.DevPodWorkspaceSourceAnnotation])
	if err != nil {
		return err
	}

	devpodWorkspace := provider.Workspace{
		ID:                id,
		UID:               workspaceUID,
		Folder:            workspaceFolder,
		Picture:           workspace.Annotations[storagev1.DevPodWorkspacePictureAnnotation],
		Provider:          providerObj,
		Machine:           provider.WorkspaceMachineConfig{},
		IDE:               provider.WorkspaceIDEConfig{},
		Source:            *providerWorkspaceSource,
		DevContainerPath:  "",
		CreationTimestamp: types.Time(workspace.CreationTimestamp),
		LastUsedTimestamp: types.Time(workspace.CreationTimestamp),
		Context:           workspaceContext,
	}

	encoded, err := json.Marshal(devpodWorkspace)
	if err != nil {
		return err
	}

	err = os.MkdirAll(workspaceFolder, 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(workspaceFolder, provider.WorkspaceConfigFile), encoded, 0666)
	if err != nil {
		return fmt.Errorf("write workspace config file: %w", err)
	}

	return nil
}

func (cmd *ImportCmd) fetchWorkspace(ctx context.Context,
	managementClient kube.Interface, workspaceUID string, projectName string) (*managementv1.DevPodWorkspaceInstance, error) {

	workspaceList, err := managementClient.
		Loft().
		ManagementV1().
		DevPodWorkspaceInstances(naming.ProjectNamespace(projectName)).
		List(ctx, metav1.ListOptions{LabelSelector: storagev1.DevPodWorkspaceUIDLabel + "=" + workspaceUID})

	if err != nil {
		return nil, err
	}

	if len(workspaceList.Items) == 0 {
		return nil, fmt.Errorf("could not find corresponding workspace")
	}

	return &workspaceList.Items[0], nil
}
