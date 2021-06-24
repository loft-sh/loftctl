package list

import (
	"context"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
loft list vcluster
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
############### devspace list vclusters ###############
#######################################################
List the loft virtual clusters you have access to

Example:
devspace list vcluster
#######################################################
	`
	}
	loginCmd := &cobra.Command{
		Use:   "vclusters",
		Short: "Lists the loft virtual clusters you have access to",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd, args)
		},
	}

	return loginCmd
}

// Run executes the functionality
func (cmd *VirtualClustersCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	userName, err := helper.GetCurrentUser(context.TODO(), managementClient)
	if err != nil {
		return err
	}

	virtualClusters, err := managementClient.Loft().ManagementV1().Users().ListVirtualClusters(context.TODO(), userName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "list users")
	}

	header := []string{
		"Name",
		"Space",
		"Cluster",
		"Status",
		"Age",
	}
	values := [][]string{}
	for _, virtualCluster := range virtualClusters.VirtualClusters {
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
