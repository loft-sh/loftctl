package create

import (
	"context"
	"github.com/loft-sh/loftctl/pkg/kube"
	"strconv"

	"github.com/loft-sh/kiosk/pkg/apis/config/v1alpha1"
	tenancyv1alpha1 "github.com/loft-sh/kiosk/pkg/apis/tenancy/v1alpha1"
	v1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	SleepAfter    int64
	DeleteAfter   int64
	Cluster       string
	Account       string
	CreateContext bool
	SwitchContext bool

	Log log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
#######################################################
################## loft create space ##################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
loft create space myspace
loft create space myspace --cluster mycluster
loft create space myspace --cluster mycluster --account myaccount
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################ devspace create space ################
#######################################################
Creates a new kube context for the given cluster, if
it does not yet exist.

Example:
devspace create space myspace
devspace create space myspace --cluster mycluster
devspace create space myspace --cluster mycluster --account myaccount
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "space",
		Short: "Creates a new space in the given cluster",
		Long:  description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVar(&cmd.Account, "account", "", "The cluster account to use")
	c.Flags().Int64Var(&cmd.SleepAfter, "sleep-after", 0, "If set to non zero, will tell the space to sleep after specified seconds of inactivity")
	c.Flags().Int64Var(&cmd.DeleteAfter, "delete-after", 0, "If set to non zero, will tell loft to delete the space after specified seconds of inactivity")
	c.Flags().BoolVar(&cmd.CreateContext, "create-context", true, "If loft should create a kube context for the space")
	c.Flags().BoolVar(&cmd.SwitchContext, "switch-context", true, "If loft should switch the current context to the new context")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(cobraCmd *cobra.Command, args []string) error {
	spaceName := args[0]
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	// determine cluster name
	clusterName := cmd.Cluster
	if clusterName == "" {
		clusterName, err = helper.SelectCluster(baseClient, cmd.Log)
		if err != nil {
			return err
		}
	}

	// determine account name
	accountName := cmd.Account
	if accountName == "" {
		accountName, err = helper.SelectAccount(baseClient, clusterName, cmd.Log)
		if err != nil {
			return err
		}
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return err
	}

	// get owner references
	ownerReferences, err := getOwnerReferences(managementClient, clusterName, accountName)
	if err != nil {
		return err
	}

	// create a cluster client
	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	// create space object
	space := &tenancyv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:            spaceName,
			Annotations:     map[string]string{},
			OwnerReferences: ownerReferences,
		},
		Spec: tenancyv1alpha1.SpaceSpec{
			Account: accountName,
		},
	}
	if cmd.SleepAfter > 0 {
		space.Annotations["sleepmode.loft.sh/sleep-after"] = strconv.FormatInt(cmd.SleepAfter, 10)
	}
	if cmd.DeleteAfter > 0 {
		space.Annotations["sleepmode.loft.sh/delete-after"] = strconv.FormatInt(cmd.DeleteAfter, 10)
	}

	// create the space
	_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Create(context.TODO(), space, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create space")
	}

	cmd.Log.Donef("Successfully created the space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// should we create a kube context for the space
	if cmd.CreateContext {
		// update kube config
		err = kubeconfig.UpdateKubeConfig(baseClient.Config(), cmd.Config, clusterName, spaceName, cmd.SwitchContext)
		if err != nil {
			return err
		}

		cmd.Log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
	}

	return nil
}

func getOwnerReferences(client kube.Interface, cluster, accountName string) ([]metav1.OwnerReference, error) {
	clusterMembers, err := client.Loft().ManagementV1().Clusters().ListMembers(context.TODO(), cluster, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	account := findOwnerAccount(append(clusterMembers.Teams, clusterMembers.Users...), accountName)

	if account != nil {
		return []metav1.OwnerReference{
			{
				APIVersion: account.APIVersion,
				Kind:       account.Kind,
				Name:       account.Name,
				UID:        account.UID,
			},
		}, nil
	}

	return []metav1.OwnerReference{}, nil
}

func findOwnerAccount(haystack []v1.ClusterMember, needle string) *v1alpha1.Account {
	for _, member := range haystack {
		if member.Account != nil && member.Account.Name == needle {
			return member.Account
		}
	}
	return nil
}
