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
	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/remotecommand"
	"github.com/spf13/cobra"
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
			return cmd.Run(context.Background())
		},
	}

	return c
}

func (cmd *UpCmd) Run(ctx context.Context) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	restConfig, err := baseClient.ManagementConfig()
	if err != nil {
		return err
	}

	host := restConfig.Host
	parsedURL, _ := url.Parse(restConfig.Host)
	if parsedURL != nil && parsedURL.Host != "" {
		host = parsedURL.Host
	}

	loftURL := "wss://" + host + "/kubernetes/management/apis/management.loft.sh/v1/namespaces/loft-p-default/devpodworkspaceinstances/test/up"
	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}

	conn, _, err := dialer.Dial(loftURL, map[string][]string{
		"Authorization": []string{"Bearer " + restConfig.BearerToken},
	})
	if err != nil {
		return fmt.Errorf("error dialing %s: %w", loftURL, err)
	}

	_, err = remotecommand.ExecuteConn(ctx, conn, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("error executing: %w", err)
	}

	return nil
}
