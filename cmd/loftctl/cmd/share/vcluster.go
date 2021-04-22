package share

import (
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

// VClusterCmd holds the cmd flags
type VClusterCmd struct {
	*flags.GlobalFlags

	Cluster         string
	Space           string
	ClusterRole     string
	User            string
	Team            string
	AllowDuplicates bool

	Log log.Logger
}

// NewVClusterCmd creates a new command
func NewVClusterCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VClusterCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
################# loft share vcluster #################
#######################################################
Shares a vcluster with another loft user or team. The 
user or team need to have access to the cluster.

Example:
loft share vcluster myvcluster
loft share vcluster myvcluster --cluster mycluster
loft share vcluster myvcluster --cluster mycluster --user admin
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace share vcluster ###############
#######################################################
Shares a vcluster with another loft user or team. The 
user or team need to have access to the cluster.

Example:
devspace share vcluster myvcluster
devspace share vcluster myvcluster --cluster mycluster
devspace share vcluster myvcluster --cluster mycluster --user admin
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "vcluster",
		Short: "Shares a vcluster with another loft user or team",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVar(&cmd.Space, "space", "", "The space to use")
	c.Flags().StringVar(&cmd.ClusterRole, "cluster-role", "loft-cluster-space-default", "The cluster role which is assigned to the user or team for that space")
	c.Flags().StringVar(&cmd.User, "user", "", "The user to share the space with. The user needs to have access to the cluster")
	c.Flags().StringVar(&cmd.Team, "team", "", "The team to share the space with. The team needs to have access to the cluster")
	c.Flags().BoolVar(&cmd.AllowDuplicates, "allow-duplicates", false, "If true multiple rolebindings are allowed for an user or team in a space")
	return c
}

// Run executes the command
func (cmd *VClusterCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	vClusterName := ""
	if len(args) > 0 {
		vClusterName = args[0]
	}

	vClusterName, spaceName, clusterName, err := helper.SelectVirtualClusterAndSpaceAndClusterName(baseClient, vClusterName, cmd.Space, cmd.Cluster, cmd.Log)
	if err != nil {
		return err
	}

	userOrTeam, err := createRoleBinding(baseClient, clusterName, spaceName, cmd.User, cmd.Team, cmd.ClusterRole, cmd.AllowDuplicates, cmd.Log)
	if err != nil {
		return err
	}

	if userOrTeam.Team == false {
		cmd.Log.Donef("Successfully granted user %s access to vCluster %s", ansi.Color(userOrTeam.ClusterMember.Info.Name, "white+b"), ansi.Color(vClusterName, "white+b"))
		cmd.Log.Infof("The user can access the vCluster now via: %s", ansi.Color(fmt.Sprintf("loft use vcluster %s --space %s --cluster %s", vClusterName, spaceName, clusterName), "white+b"))
	} else {
		cmd.Log.Donef("Successfully granted team %s access to vCluster %s", ansi.Color(userOrTeam.ClusterMember.Info.Name, "white+b"), ansi.Color(vClusterName, "white+b"))
		cmd.Log.Infof("The team can access the vCluster now via: %s", ansi.Color(fmt.Sprintf("loft use vcluster %s --space %s --cluster %s", vClusterName, spaceName, clusterName), "white+b"))
	}

	return nil
}
