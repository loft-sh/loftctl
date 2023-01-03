package importcmd

import (
	"context"

	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/mgutz/ansi"

	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"

	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VClusterCmd struct {
	*flags.GlobalFlags

	VClusterClusterName string
	VClusterNamespace   string
	Project             string
	ImportName          string

	log log.Logger
}

// NewVClusterCmd creates a new command
func NewVClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VClusterCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	description := `
#######################################################
################## loft import vcluster ###############
#######################################################
Imports a vcluster into a Loft project.

Example:
loft import vcluster my-vcluster --cluster connected-cluster my-vcluster \
  --namespace vcluster-my-vcluster --project my-project --importname my-vcluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace import vcluster #############
#######################################################
Imports a vcluster into a Loft project.

Example:
devspace import vcluster my-vcluster --cluster connected-cluster my-vcluster \
  --namespace vcluster-my-vcluster --project my-project --importname my-vcluster
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Imports a vcluster into a Loft project",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(args)
		},
	}

	c.Flags().StringVar(&cmd.VClusterClusterName, "cluster", "", "Cluster name of the cluster the virtual cluster is running on")
	c.Flags().StringVar(&cmd.VClusterNamespace, "namespace", "", "The namespace of the vcluster")
	c.Flags().StringVar(&cmd.Project, "project", "", "The project to import the vcluster into")
	c.Flags().StringVar(&cmd.ImportName, "importname", "", "The name of the vcluster under projects. If unspecified, will use the vcluster name")

	c.MarkFlagRequired("cluster")
	c.MarkFlagRequired("namespace")
	c.MarkFlagRequired("project")

	return c
}

func (cmd *VClusterCmd) Run(args []string) error {
	// Get vclusterName from command argument
	var vclusterName string = args[0]

	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	err = client.VerifyVersion(baseClient)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	if _, err = managementClient.Loft().ManagementV1().Projects().ImportVirtualCluster(context.TODO(), cmd.Project, &managementv1.ProjectImportVirtualCluster{
		SourceVirtualCluster: managementv1.ProjectImportVirtualClusterSource{
			Name:       vclusterName,
			Namespace:  cmd.VClusterNamespace,
			Cluster:    cmd.VClusterClusterName,
			ImportName: cmd.ImportName,
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}

	cmd.log.Donef("Successfully imported vcluster %s into project %s", ansi.Color(vclusterName, "white+b"), ansi.Color(cmd.Project, "white+b"))

	return nil
}
