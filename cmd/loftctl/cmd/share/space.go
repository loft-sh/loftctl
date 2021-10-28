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

	Cluster     string
	ClusterRole string
	User        string
	Team        string

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
	c.Flags().StringVar(&cmd.ClusterRole, "cluster-role", "loft-cluster-space-admin", "The cluster role which is assigned to the user or team for that space")
	c.Flags().StringVar(&cmd.User, "user", "", "The user to share the space with. The user needs to have access to the cluster")
	c.Flags().StringVar(&cmd.Team, "team", "", "The team to share the space with. The team needs to have access to the cluster")
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

	userOrTeam, err := createRoleBinding(baseClient, clusterName, spaceName, cmd.User, cmd.Team, cmd.ClusterRole, cmd.Log)
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

func createRoleBinding(baseClient client.Client, clusterName, spaceName, userName, teamName, clusterRole string, log log.Logger) (*helper.ClusterUserOrTeam, error) {
	userOrTeam, err := helper.SelectClusterUserOrTeam(baseClient, clusterName, userName, teamName, log)
	if err != nil {
		return nil, err
	}

	clusterClient, err := baseClient.Cluster(clusterName)
	if err != nil {
		return nil, err
	}

	// check if there is already a role binding for this user or team already
	roleBindings, err := clusterClient.RbacV1().RoleBindings(spaceName).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	subjectString := ""
	if userOrTeam.Team {
		subjectString = "loft:team:"+userOrTeam.ClusterMember.Info.Name
	} else {
		subjectString = "loft:user:"+userOrTeam.ClusterMember.Info.Name
	}
	
	// check if there is already a role binding
	for _, roleBinding := range roleBindings.Items {
		if roleBinding.RoleRef.Kind == "ClusterRole" && roleBinding.RoleRef.Name == clusterRole {
			for _, subject := range roleBinding.Subjects {
				if subject.Kind == "Group" || subject.Kind == "User" {
					if subject.Name == subjectString {
						return nil, nil
					}
				}
			}
		}
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
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "Group",
				APIGroup: rbacv1.GroupName,
				Name: subjectString,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "create rolebinding")
	}

	return userOrTeam, nil
}
