package create

import (
	"context"

	"github.com/kiosk-sh/kiosk/pkg/apis/config/v1alpha1"
	tenancyv1alpha1 "github.com/kiosk-sh/kiosk/pkg/apis/tenancy/v1alpha1"
	v1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/kube"
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

	Cluster       string
	Account       string
	CreateContext bool
	SwitchContext bool

	log log.Logger
}

// NewSpaceCmd creates a new command
func NewSpaceCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &SpaceCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
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
			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVar(&cmd.Account, "account", "", "The cluster account to use")
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
		clusterName, err = helper.SelectCluster(baseClient, cmd.log)
		if err != nil {
			return err
		}
	}

	// determine account name
	accountName := cmd.Account
	if accountName == "" {
		accountName, err = helper.SelectAccount(baseClient, clusterName, cmd.log)
		if err != nil {
			return err
		}
	}

	// create a cluster client
	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return err
	}

	// get owner references
	ownerReferences, err := getOwnerReferences(clusterClient, clusterName, accountName)
	if err != nil {
		return err
	}

	// create the space
	_, err = clusterClient.Kiosk().TenancyV1alpha1().Spaces().Create(context.TODO(), &tenancyv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:            spaceName,
			OwnerReferences: ownerReferences,
		},
		Spec: tenancyv1alpha1.SpaceSpec{
			Account: accountName,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create space")
	}

	cmd.log.Donef("Successfully created the space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))

	// should we create a kube context for the space
	if cmd.CreateContext {
		// update kube config
		err = kubeconfig.UpdateKubeConfig(baseClient.Config(), clusterName, spaceName, cmd.SwitchContext)
		if err != nil {
			return err
		}

		cmd.log.Donef("Successfully updated kube context to use space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
	}

	return nil
}

func getOwnerReferences(clusterClient kube.Interface, cluster, accountName string) ([]metav1.OwnerReference, error) {
	clusterMembers, err := clusterClient.Loft().ManagementV1().Clusters().ListMembers(context.TODO(), cluster, metav1.GetOptions{})
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
