package devpod

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v3/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/naming"
	"github.com/loft-sh/loftctl/v3/pkg/remotecommand"
	"github.com/loft-sh/log"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// UpCmd holds the cmd flags
type UpCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewUpCmd creates a new command
func NewUpCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &UpCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	c := &cobra.Command{
		Use:   "up",
		Short: "Runs up on a workspace",
		Long: `
#######################################################
#################### loft devpod up ###################
#######################################################
	`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(context.Background(), log.GetInstance().ErrorStreamOnly())
		},
	}

	return c
}

func (cmd *UpCmd) Run(ctx context.Context, log log.Logger) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	workspace, err := findWorkspace(ctx, baseClient)
	if err != nil {
		return err
	}

	// create workspace if doesn't exist
	if workspace == nil {
		workspace, err = createWorkspace(ctx, baseClient, log)
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
	}

	conn, err := dialWorkspace(baseClient, workspace, "up", optionsFromEnv(storagev1.DevPodFlagsUp))
	if err != nil {
		return err
	}

	_, err = remotecommand.ExecuteConn(ctx, conn, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("error executing: %w", err)
	}

	return nil
}

func createWorkspace(ctx context.Context, baseClient client.Client, log log.Logger) (*managementv1.DevPodWorkspaceInstance, error) {
	workspaceID, workspaceUID, projectName, err := getWorkspaceInfo()
	if err != nil {
		return nil, err
	}

	// get template
	template := os.Getenv(LOFT_TEMPLATE_OPTION)
	if template == "" {
		return nil, fmt.Errorf("%s is missing in environment", LOFT_TEMPLATE_OPTION)
	}

	workspace := &managementv1.DevPodWorkspaceInstance{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: naming.SafeConcatNameMax([]string{workspaceID}, 53) + "-",
			Namespace:    naming.ProjectNamespace(projectName),
			Labels: map[string]string{
				storagev1.DevPodWorkspaceIDLabel:  workspaceID,
				storagev1.DevPodWorkspaceUIDLabel: workspaceUID,
			},
		},
		Spec: managementv1.DevPodWorkspaceInstanceSpec{
			DevPodWorkspaceInstanceSpec: storagev1.DevPodWorkspaceInstanceSpec{
				DisplayName: workspaceID,
				TemplateRef: &storagev1.TemplateRef{
					Name: template,
				},
			},
		},
	}
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, err
	}

	// create instance
	workspace, err = managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(naming.ProjectNamespace(projectName)).Create(ctx, workspace, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	log.Infof("Created workspace %s", workspace.Name)

	// we need to wait until instance is scheduled
	err = wait.PollImmediate(time.Second, time.Second*30, func() (bool, error) {
		workspace, err = managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(naming.ProjectNamespace(projectName)).Get(ctx, workspace.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if !isReady(workspace) {
			log.Debugf("Workspace %s is in phase %s, waiting until its ready", workspace.Name, workspace.Status.Phase)
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait for instance to get ready: %w", err)
	}

	return workspace, nil
}

func isReady(workspace *managementv1.DevPodWorkspaceInstance) bool {
	return workspace.Status.Phase == storagev1.InstanceReady
}

func getWorkspaceInfo() (string, string, string, error) {
	// get workspace id
	workspaceID := os.Getenv(LOFT_WORKSPACE_ID)
	if workspaceID == "" {
		return "", "", "", fmt.Errorf("%s is missing in environment", LOFT_WORKSPACE_ID)
	}

	// get workspace uid
	workspaceUID := os.Getenv(LOFT_WORKSPACE_UID)
	if workspaceUID == "" {
		return "", "", "", fmt.Errorf("%s is missing in environment", LOFT_WORKSPACE_UID)
	}

	// get project
	projectName := os.Getenv(LOFT_PROJECT_OPTION)
	if projectName == "" {
		return "", "", "", fmt.Errorf("%s is missing in environment", LOFT_PROJECT_OPTION)
	}

	return workspaceID, workspaceUID, projectName, nil
}

func findWorkspace(ctx context.Context, baseClient client.Client) (*managementv1.DevPodWorkspaceInstance, error) {
	_, workspaceUID, projectName, err := getWorkspaceInfo()
	if err != nil {
		return nil, err
	}

	// create client
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, fmt.Errorf("create management client: %w", err)
	}

	// get workspace
	workspaceList, err := managementClient.Loft().ManagementV1().DevPodWorkspaceInstances(naming.ProjectNamespace(projectName)).List(ctx, metav1.ListOptions{
		LabelSelector: storagev1.DevPodWorkspaceUIDLabel + "=" + workspaceUID,
	})
	if err != nil {
		return nil, err
	} else if len(workspaceList.Items) == 0 {
		return nil, nil
	}

	return &workspaceList.Items[0], nil
}

func optionsFromEnv(name string) url.Values {
	options := os.Getenv(name)
	if options != "" {
		return url.Values{
			"options": []string{options},
		}
	}

	return nil
}

func dialWorkspace(baseClient client.Client, workspace *managementv1.DevPodWorkspaceInstance, subResource string, values url.Values) (*websocket.Conn, error) {
	restConfig, err := baseClient.ManagementConfig()
	if err != nil {
		return nil, err
	}

	host := restConfig.Host
	parsedURL, _ := url.Parse(restConfig.Host)
	if parsedURL != nil && parsedURL.Host != "" {
		host = parsedURL.Host
	}

	loftURL := "wss://" + host + "/kubernetes/management/apis/management.loft.sh/v1/namespaces/" + workspace.Namespace + "/devpodworkspaceinstances/" + workspace.Name + "/" + subResource
	if len(values) > 0 {
		loftURL += "?" + values.Encode()
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}

	conn, _, err := dialer.Dial(loftURL, map[string][]string{
		"Authorization": {"Bearer " + restConfig.BearerToken},
	})
	if err != nil {
		return nil, fmt.Errorf("error dialing %s: %w", loftURL, err)
	}

	return conn, nil
}
