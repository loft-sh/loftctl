package helper

import (
	"context"
	"fmt"
	clusterv1 "github.com/loft-sh/agentapi/pkg/apis/loft/cluster/v1"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/clihelper"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/kubeconfig"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sort"
	"strings"
)

// SelectCluster lets the user select a cluster
func SelectCluster(baseClient client.Client, log log.Logger) (string, error) {
	managementClient, err := baseClient.Management()
	if err != nil {
		return "", err
	}

	clusterList, err := managementClient.Loft().ManagementV1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	clusterNames := []string{}
	for _, cluster := range clusterList.Items {
		clusterNames = append(clusterNames, clihelper.GetDisplayName(cluster.Name, cluster.Spec.DisplayName))
	}

	if len(clusterNames) == 0 {
		return "", fmt.Errorf("the user has no access to any cluster")
	} else if len(clusterNames) == 1 {
		return clusterList.Items[0].Name, nil
	}

	answer, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose a cluster to use",
		DefaultValue: clusterNames[0],
		Options:      clusterNames,
	})
	if err != nil {
		return "", err
	}
	for _, cluster := range clusterList.Items {
		if answer == clihelper.GetDisplayName(cluster.Name, cluster.Spec.DisplayName) {
			return cluster.Name, nil
		}
	}
	return "", fmt.Errorf("answer not found")
}

// SelectUserOrTeam lets the user select an user or team in a cluster
func SelectUserOrTeam(baseClient client.Client, clusterName string, log log.Logger) (*clusterv1.EntityInfo, *clusterv1.EntityInfo, error) {
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, nil, err
	}

	clusterAccess, err := managementClient.Loft().ManagementV1().Clusters().ListAccess(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	var user *clusterv1.EntityInfo
	if len(clusterAccess.Users) > 0 {
		user = &clusterAccess.Users[0].Info
	}

	teams := []*clusterv1.EntityInfo{}
	for _, team := range clusterAccess.Teams {
		t := team
		teams = append(teams, &t.Info)
	}

	if user == nil && len(teams) == 0 {
		return nil, nil, fmt.Errorf("the user has no access to cluster %s", clusterName)
	} else if user != nil && len(teams) == 0 {
		return user, nil, nil
	} else if user == nil && len(teams) == 1 {
		return nil, teams[0], nil
	}

	names := []string{}
	if user != nil {
		names = append(names, "User "+clihelper.DisplayName(user))
	}
	for _, t := range teams {
		names = append(names, "Team "+clihelper.DisplayName(t))
	}

	answer, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose an user or team to use",
		DefaultValue: names[0],
		Options:      names,
	})
	if err != nil {
		return nil, nil, err
	}

	if user != nil && "User "+clihelper.DisplayName(user) == answer {
		return user, nil, nil
	}
	for _, t := range teams {
		if "Team "+clihelper.DisplayName(t) == answer {
			return nil, t, nil
		}
	}

	return nil, nil, fmt.Errorf("answer not found")
}

func SelectPod(client kubernetes.Interface, namespace string, log log.Logger) (string, error) {
	podList, err := client.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app=vcluster",
	})
	if err != nil {
		return "", err
	}

	options := []string{}
	for _, pod := range podList.Items {
		options = append(options, pod.Name)
	}

	if len(options) == 0 {
		return "", fmt.Errorf("no virtual cluster found in namespace %s", namespace)
	} else if len(options) == 1 {
		return options[0], nil
	}

	selectedPod, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose a virtual cluster pod",
		DefaultValue: options[0],
		Options:      options,
	})
	if err != nil {
		return "", err
	}

	return selectedPod, nil
}

type ClusterUserOrTeam struct {
	Team          bool
	ClusterMember managementv1.ClusterMember
}

func SelectClusterUserOrTeam(baseClient client.Client, clusterName, userName, teamName string, log log.Logger) (*ClusterUserOrTeam, error) {
	if userName != "" && teamName != "" {
		return nil, fmt.Errorf("team and user specified, please only choose one")
	}

	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, err
	}

	members, err := managementClient.Loft().ManagementV1().Clusters().ListMembers(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "retrieve cluster members")
	}

	matchedMembers := []ClusterUserOrTeam{}
	optionsUnformatted := [][]string{}
	for _, user := range members.Users {
		if teamName != "" {
			continue
		} else if userName != "" && user.Info.Name != userName {
			continue
		}

		matchedMembers = append(matchedMembers, ClusterUserOrTeam{
			ClusterMember: user,
		})
		displayName := user.Info.DisplayName
		if displayName == "" {
			displayName = user.Info.Name
		}

		optionsUnformatted = append(optionsUnformatted, []string{"User: " + displayName, "Kube User: " + user.Info.Name})
	}
	for _, team := range members.Teams {
		if userName != "" {
			continue
		} else if teamName != "" && team.Info.Name != teamName {
			continue
		}

		matchedMembers = append(matchedMembers, ClusterUserOrTeam{
			Team:          true,
			ClusterMember: team,
		})
		displayName := team.Info.DisplayName
		if displayName == "" {
			displayName = team.Info.Name
		}

		optionsUnformatted = append(optionsUnformatted, []string{"Team: " + displayName, "Kube Team: " + team.Info.Name})
	}

	questionOptions := formatOptions("%s | %s", optionsUnformatted)
	if len(questionOptions) == 0 {
		if userName == "" && teamName == "" {
			return nil, fmt.Errorf("couldn't find any space")
		} else if userName != "" {
			return nil, fmt.Errorf("couldn't find user %s in cluster %s", ansi.Color(userName, "white+b"), ansi.Color(clusterName, "white+b"))
		}

		return nil, fmt.Errorf("couldn't find team %s in cluster %s", ansi.Color(teamName, "white+b"), ansi.Color(clusterName, "white+b"))
	} else if len(questionOptions) == 1 {
		return &matchedMembers[0], nil
	}

	selectedMember, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose an user or team",
		DefaultValue: questionOptions[0],
		Options:      questionOptions,
	})
	if err != nil {
		return nil, err
	}

	for idx, s := range questionOptions {
		if s == selectedMember {
			return &matchedMembers[idx], nil
		}
	}

	return nil, fmt.Errorf("selected question option not found")
}

type ClusterSpace struct {
	clusterv1.Space
	Cluster string
}

// GetSpaces returns all spaces accessible by the user or team
func GetSpaces(baseClient client.Client) ([]ClusterSpace, error) {
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, err
	}

	clusterList, err := managementClient.Loft().ManagementV1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	spaceList := []ClusterSpace{}
	for _, cluster := range clusterList.Items {
		clusterClient, err := baseClient.Cluster(cluster.Name)
		if err != nil {
			return nil, err
		}

		spaces, err := clusterClient.Agent().ClusterV1().Spaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, space := range spaces.Items {
			spaceList = append(spaceList, ClusterSpace{
				Space:   space,
				Cluster: cluster.Name,
			})
		}
	}
	sort.Slice(spaceList, func(i, j int) bool {
		return spaceList[i].Name < spaceList[j].Name
	})

	return spaceList, nil
}

type ClusterVirtualCluster struct {
	clusterv1.VirtualCluster
	Cluster string
}

// GetVirtualClusters returns all virtual clusters the user has access to
func GetVirtualClusters(baseClient client.Client) ([]ClusterVirtualCluster, error) {
	managementClient, err := baseClient.Management()
	if err != nil {
		return nil, err
	}

	clusterList, err := managementClient.Loft().ManagementV1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	virtualClusterList := []ClusterVirtualCluster{}
	for _, cluster := range clusterList.Items {
		clusterClient, err := baseClient.Cluster(cluster.Name)
		if err != nil {
			return nil, err
		}

		virtualClusters, err := clusterClient.Agent().ClusterV1().VirtualClusters("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, virtualCluster := range virtualClusters.Items {
			virtualClusterList = append(virtualClusterList, ClusterVirtualCluster{
				VirtualCluster: virtualCluster,
				Cluster:        cluster.Name,
			})
		}
	}
	sort.Slice(virtualClusterList, func(i, j int) bool {
		return virtualClusterList[i].Name < virtualClusterList[j].Name
	})

	return virtualClusterList, nil
}

// SelectSpaceAndClusterName selects a space and cluster name
func SelectSpaceAndClusterName(baseClient client.Client, spaceName, clusterName string, log log.Logger) (string, string, error) {
	spaces, err := GetSpaces(baseClient)
	if err != nil {
		return "", "", err
	}

	currentContext, err := kubeconfig.CurrentContext()
	if err != nil {
		return "", "", errors.Wrap(err, "loading kubernetes config")
	}

	isLoftContext, cluster, namespace, vCluster := kubeconfig.ParseContext(currentContext)
	matchedSpaces := []ClusterSpace{}
	questionOptionsUnformatted := [][]string{}
	defaultIndex := 0
	for _, space := range spaces {
		if spaceName != "" && space.Space.Name != spaceName {
			continue
		} else if clusterName != "" && space.Cluster != clusterName {
			continue
		}

		if isLoftContext == true && vCluster == "" && cluster == space.Cluster && namespace == space.Space.Name {
			defaultIndex = len(questionOptionsUnformatted)
		}

		matchedSpaces = append(matchedSpaces, space)
		questionOptionsUnformatted = append(questionOptionsUnformatted, []string{space.Space.Name, space.Cluster})
	}

	questionOptions := formatOptions("Space: %s | Cluster: %s", questionOptionsUnformatted)
	if len(questionOptions) == 0 {
		if spaceName == "" {
			return "", "", fmt.Errorf("couldn't find any space")
		} else if clusterName != "" {
			return "", "", fmt.Errorf("couldn't find space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
		}

		return "", "", fmt.Errorf("couldn't find space %s", ansi.Color(spaceName, "white+b"))
	} else if len(questionOptions) == 1 {
		return matchedSpaces[0].Space.Name, matchedSpaces[0].Cluster, nil
	}

	selectedSpace, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose a space to use",
		DefaultValue: questionOptions[defaultIndex],
		Options:      questionOptions,
	})
	if err != nil {
		return "", "", err
	}

	for idx, s := range questionOptions {
		if s == selectedSpace {
			clusterName = matchedSpaces[idx].Cluster
			spaceName = matchedSpaces[idx].Space.Name
			break
		}
	}

	return spaceName, clusterName, nil
}

func GetCurrentUser(ctx context.Context, managementClient kube.Interface) (*managementv1.UserInfo, *clusterv1.EntityInfo, error) {
	self, err := managementClient.Loft().ManagementV1().Selves().Create(ctx, &managementv1.Self{}, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "get self")
	} else if self.Status.User == nil && self.Status.Team == nil {
		return nil, nil, fmt.Errorf("no user or team name returned")
	}

	return self.Status.User, self.Status.Team, nil
}

func SelectVirtualClusterAndSpaceAndClusterName(baseClient client.Client, virtualClusterName, spaceName, clusterName string, log log.Logger) (string, string, string, error) {
	virtualClusters, err := GetVirtualClusters(baseClient)
	if err != nil {
		return "", "", "", err
	}

	currentContext, err := kubeconfig.CurrentContext()
	if err != nil {
		return "", "", "", errors.Wrap(err, "loading kubernetes config")
	}

	isLoftContext, cluster, namespace, vCluster := kubeconfig.ParseContext(currentContext)
	matchedVClusters := []ClusterVirtualCluster{}
	questionOptionsUnformatted := [][]string{}
	defaultIndex := 0
	for _, virtualCluster := range virtualClusters {
		if virtualClusterName != "" && virtualCluster.VirtualCluster.Name != virtualClusterName {
			continue
		} else if spaceName != "" && virtualCluster.VirtualCluster.Namespace != spaceName {
			continue
		} else if clusterName != "" && virtualCluster.Cluster != clusterName {
			continue
		}

		if isLoftContext == true && vCluster == virtualCluster.VirtualCluster.Name && cluster == virtualCluster.Cluster && namespace == virtualCluster.VirtualCluster.Namespace {
			defaultIndex = len(questionOptionsUnformatted)
		}

		matchedVClusters = append(matchedVClusters, virtualCluster)
		questionOptionsUnformatted = append(questionOptionsUnformatted, []string{virtualCluster.VirtualCluster.Name, virtualCluster.VirtualCluster.Namespace, virtualCluster.Cluster})
	}

	questionOptions := formatOptions("vCluster: %s | Space: %s | Cluster: %s", questionOptionsUnformatted)
	if len(questionOptions) == 0 {
		if virtualClusterName == "" {
			return "", "", "", fmt.Errorf("couldn't find any virtual cluster")
		} else if spaceName != "" {
			return "", "", "", fmt.Errorf("couldn't find virtualcluster %s in space %s in cluster %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
		} else if clusterName != "" {
			return "", "", "", fmt.Errorf("couldn't find virtualcluster %s in space %s in cluster %s", ansi.Color(virtualClusterName, "white+b"), ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
		}

		return "", "", "", fmt.Errorf("couldn't find virtual cluster %s", ansi.Color(virtualClusterName, "white+b"))
	} else if len(questionOptions) == 1 {
		return matchedVClusters[0].VirtualCluster.Name, matchedVClusters[0].VirtualCluster.Namespace, matchedVClusters[0].Cluster, nil
	}

	selectedSpace, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose a virtual cluster to use",
		DefaultValue: questionOptions[defaultIndex],
		Options:      questionOptions,
	})
	if err != nil {
		return "", "", "", err
	}

	for idx, s := range questionOptions {
		if s == selectedSpace {
			clusterName = matchedVClusters[idx].Cluster
			virtualClusterName = matchedVClusters[idx].VirtualCluster.Name
			spaceName = matchedVClusters[idx].VirtualCluster.Namespace
			break
		}
	}

	return virtualClusterName, spaceName, clusterName, nil
}

func formatOptions(format string, options [][]string) []string {
	if len(options) == 0 {
		return []string{}
	}

	columnLengths := make([]int, len(options[0]))
	for _, row := range options {
		for i, column := range row {
			if len(column) > columnLengths[i] {
				columnLengths[i] = len(column)
			}
		}
	}

	retOptions := []string{}
	for _, row := range options {
		columns := []interface{}{}
		for i := range row {
			value := row[i]
			if columnLengths[i] > len(value) {
				value = value + strings.Repeat(" ", columnLengths[i]-len(value))
			}

			columns = append(columns, value)
		}

		retOptions = append(retOptions, fmt.Sprintf(format, columns...))
	}

	return retOptions
}
