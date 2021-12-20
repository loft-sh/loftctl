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

// ClustersCmd holds the login cmd flags
type ClustersCmd struct {
	*flags.GlobalFlags

	log log.Logger
}

// NewClustersCmd creates a new spaces command
func NewClustersCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &ClustersCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}
	description := `
#######################################################
################## loft list clusters #################
#######################################################
List the loft clusters you have access to

Example:
loft list clusters
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace list clusters ################
#######################################################
List the loft clusters you have access to

Example:
devspace list clusters
#######################################################
	`
	}
	clustersCmd := &cobra.Command{
		Use:   "clusters",
		Short: "Lists the loft clusters you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.RunClusters(cobraCmd, args)
		},
	}

	return clustersCmd
}

// RunUsers executes the functionality "loft list users"
func (cmd *ClustersCmd) RunClusters(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	clusters, err := helper.ListClusterAccounts(baseClient)
	if err != nil {
		return err
	}

	header := []string{
		"Cluster",
		"Account",
		"Age",
	}
	values := [][]string{}
	for _, cluster := range clusters {
		if len(cluster.Accounts) == 0 {
			continue
		}

		values = append(values, []string{
			cluster.Cluster.Name,
			cluster.Accounts[0],
			duration.HumanDuration(time.Now().Sub(cluster.Cluster.CreationTimestamp.Time)),
		})
	}

	log.PrintTable(cmd.log, header, values)
	return nil
}
