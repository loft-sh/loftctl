package list

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/duration"
	"time"
)

// VirtualClustersCmd holds the data
type VirtualClustersCmd struct {
	*flags.GlobalFlags

	log log.Logger
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
	loginCmd := &cobra.Command{
		Use:   "vclusters",
		Short: "Lists the loft virtual clusters you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run()
		},
	}

	return loginCmd
}

// Run executes the functionality
func (cmd *VirtualClustersCmd) Run() error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	virtualClusters, err := helper.GetVirtualClusters(baseClient, cmd.log)
	if err != nil {
		return err
	}

	header := []string{
		"Name",
		"Space",
		"Cluster",
		"Status",
		"Age",
	}
	values := [][]string{}
	for _, virtualCluster := range virtualClusters {
		status := "Active"
		if virtualCluster.VirtualCluster.Status.HelmRelease != nil {
			status = string(virtualCluster.VirtualCluster.Status.HelmRelease.Phase)
		}

		values = append(values, []string{
			virtualCluster.VirtualCluster.Name,
			virtualCluster.VirtualCluster.Namespace,
			virtualCluster.Cluster,
			status,
			duration.HumanDuration(time.Now().Sub(virtualCluster.VirtualCluster.CreationTimestamp.Time)),
		})
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
