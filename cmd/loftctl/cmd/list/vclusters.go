package list

import (
	"time"

	"github.com/loft-sh/loftctl/v3/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/v3/pkg/client"
	"github.com/loft-sh/loftctl/v3/pkg/client/helper"
	"github.com/loft-sh/loftctl/v3/pkg/clihelper"
	"github.com/loft-sh/loftctl/v3/pkg/log"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/duration"
)

// VirtualClustersCmd holds the data
type VirtualClustersCmd struct {
	*flags.GlobalFlags

	ShowLegacy bool
	log        log.Logger
}

// NewVirtualClustersCmd creates a new command
func NewVirtualClustersCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &VirtualClustersCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################# loft list vclusters #################
#######################################################
List the loft virtual clusters you have access to

Example:
loft list vclusters
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace list vclusters ###############
#######################################################
List the loft virtual clusters you have access to

Example:
devspace list vclusters
#######################################################
	`
	}
	listCmd := &cobra.Command{
		Use:   "vclusters",
		Short: "Lists the loft virtual clusters you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run()
		},
	}
	listCmd.Flags().BoolVar(&cmd.ShowLegacy, "show-legacy", false, "If true, will always show the legacy virtual clusters as well")
	return listCmd
}

// Run executes the functionality
func (cmd *VirtualClustersCmd) Run() error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	header := []string{
		"Name",
		"Project",
		"Cluster",
		"Namespace",
		"Status",
		"Age",
	}
	values := [][]string{}

	virtualClusterInstances, err := helper.GetVirtualClusterInstances(baseClient)
	if err != nil {
		return err
	}

	for _, virtualCluster := range virtualClusterInstances {
		values = append(values, []string{
			clihelper.GetTableDisplayName(virtualCluster.VirtualClusterInstance.Name, virtualCluster.VirtualClusterInstance.Spec.DisplayName),
			virtualCluster.Project,
			virtualCluster.VirtualClusterInstance.Spec.ClusterRef.Cluster,
			virtualCluster.VirtualClusterInstance.Spec.ClusterRef.Namespace,
			string(virtualCluster.VirtualClusterInstance.Status.Phase),
			duration.HumanDuration(time.Since(virtualCluster.VirtualClusterInstance.CreationTimestamp.Time)),
		})
	}
	if len(virtualClusterInstances) == 0 || cmd.ShowLegacy {
		virtualClusters, err := helper.GetVirtualClusters(baseClient, cmd.log)
		if err != nil {
			return err
		}
		for _, virtualCluster := range virtualClusters {
			status := "Active"
			if virtualCluster.VirtualCluster.Status.HelmRelease != nil {
				status = virtualCluster.VirtualCluster.Status.HelmRelease.Phase
			}
			vClusterName := virtualCluster.VirtualCluster.Name
			if virtualCluster.VirtualCluster.Annotations != nil && virtualCluster.VirtualCluster.Annotations["loft.sh/display-name"] != "" {
				vClusterName = virtualCluster.VirtualCluster.Annotations["loft.sh/display-name"] + " (" + vClusterName + ")"
			}

			values = append(values, []string{
				vClusterName,
				"",
				virtualCluster.Cluster,
				virtualCluster.VirtualCluster.Namespace,
				status,
				duration.HumanDuration(time.Since(virtualCluster.VirtualCluster.CreationTimestamp.Time)),
			})
		}
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
