package share

import (
	"context"
	"fmt"
	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/client/helper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpaceCmd holds the cmd flags
type SpaceCmd struct {
	*flags.GlobalFlags

	Cluster         string
	ClusterRole     string
	User            string
	Team            string
	AllowDuplicates bool

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
################### loft share space ##################
#######################################################
Shares a space with another loft user or team. The user
or team need to have access to the cluster.

Example:
loft share space myspace
loft share space myspace --cluster mycluster
loft share space myspace --cluster mycluster --user admin
#######################################################
	`
	if upgrade.IsPlugin == "true" {
		description = `
#######################################################
################# devspace share space ################
#######################################################
Shares a space with another loft user or team. The user
or team need to have access to the cluster.

Example:
devspace share space myspace
devspace share space myspace --cluster mycluster
devspace share space myspace --cluster mycluster --user admin
#######################################################
	`
	}
	c := &cobra.Command{
		Use:   "space",
		Short: "Shares a space with another loft user or team",
		Long:  description,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	c.Flags().StringVar(&cmd.Cluster, "cluster", "", "The cluster to use")
	c.Flags().StringVar(&cmd.ClusterRole, "cluster-role", "loft-cluster-space-default", "The cluster role which is assigned to the user or team for that space")
	c.Flags().StringVar(&cmd.User, "user", "", "The user to share the space with. The user needs to have access to the cluster")
	c.Flags().StringVar(&cmd.Team, "team", "", "The team to share the space with. The team needs to have access to the cluster")
	c.Flags().BoolVar(&cmd.AllowDuplicates, "allow-duplicates", false, "If true multiple rolebindings are allowed for an user or team in a space")
	return c
}

// Run executes the command
func (cmd *SpaceCmd) Run(cobraCmd *cobra.Command, args []string) error {
	baseClient, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}

	spaceName := ""
	if len(args) > 0 {
		spaceName = args[0]
	}

	spaceName, clusterName, err := helper.SelectSpaceAndClusterName(baseClient, spaceName, cmd.Cluster, cmd.Log)
	if err != nil {
		return err
	}

	userOrTeam, err := createRoleBinding(baseClient, clusterName, spaceName, cmd.User, cmd.Team, cmd.ClusterRole, cmd.AllowDuplicates, cmd.Log)
	if err != nil {
		return err
	}

	if userOrTeam.Team == false {
		cmd.Log.Donef("Successfully granted user %s access to space %s", ansi.Color(userOrTeam.ClusterMember.Info.Name, "white+b"), ansi.Color(spaceName, "white+b"))
		cmd.Log.Infof("The user can access the space now via: %s", ansi.Color(fmt.Sprintf("loft use space %s --cluster %s", spaceName, clusterName), "white+b"))
	} else {
		cmd.Log.Donef("Successfully granted team %s access to space %s", ansi.Color(userOrTeam.ClusterMember.Info.Name, "white+b"), ansi.Color(spaceName, "white+b"))
		cmd.Log.Infof("The team can access the space now via: %s", ansi.Color(fmt.Sprintf("loft use space %s --cluster %s", spaceName, clusterName), "white+b"))
	}

	return nil
}

func createRoleBinding(baseClient client.Client, clusterName, spaceName, userName, teamName, clusterRole string, allowDuplicates bool, log log.Logger) (*helper.ClusterUserOrTeam, error) {
	userOrTeam, err := helper.SelectClusterUserOrTeam(baseClient, clusterName, userName, teamName, log)
	if err != nil {
		return nil, err
	}

	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return nil, err
	}

	// check if there is already a role binding for this user or team already
	if allowDuplicates == false {
		roleBindings, err := clusterClient.RbacV1().RoleBindings(spaceName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, roleBinding := range roleBindings.Items {
			for _, ownerReference := range roleBinding.OwnerReferences {
				if ownerReference.APIVersion == userOrTeam.ClusterMember.Account.APIVersion && ownerReference.Kind == "Account" && ownerReference.Name == userOrTeam.ClusterMember.Account.Name {
					return nil, fmt.Errorf("%s already has access to space %s. Run with --allow-duplicates to disable this check and create the rolebinding anyway", userOrTeam.ClusterMember.Info.Name, spaceName)
				}
			}
		}
	}

	// get owner references
	t := true
	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion: userOrTeam.ClusterMember.Account.APIVersion,
			Kind:       userOrTeam.ClusterMember.Account.Kind,
			Controller: &t,
			Name:       userOrTeam.ClusterMember.Account.Name,
			UID:        userOrTeam.ClusterMember.Account.UID,
		},
	}

	roleBindingName := "loft-user-" + userOrTeam.ClusterMember.Info.Name
	if userOrTeam.Team {
		roleBindingName = "loft-team-" + userOrTeam.ClusterMember.Info.Name
	}
	if len(roleBindingName) > 52 {
		roleBindingName = roleBindingName[:52]
	}

	// create the rolebinding
	_, err = clusterClient.RbacV1().RoleBindings(spaceName).Create(context.TODO(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    roleBindingName + "-",
			OwnerReferences: ownerReferences,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "create rolebinding")
	}

	return userOrTeam, nil
}
