package helper

import (
	"context"
	"fmt"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/mgutz/ansi"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ListClusterAccounts lists all the clusters and the corresponding accounts for the current user
func ListClusterAccounts(client client.Client) ([]managementv1.ClusterAccounts, error) {
	authInfo, err := client.AuthInfo()
	if err != nil {
		return nil, errors.Wrap(err, "auth info")
	}

	mClient, err := client.Management()
	if err != nil {
		return nil, err
	}

	userClusters, err := mClient.Loft().ManagementV1().Users().ListClusters(context.TODO(), authInfo.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get user")
	}

	return userClusters.Clusters, nil
}

// SelectCluster lets the user select a cluster
func SelectCluster(baseClient client.Client, log log.Logger) (string, error) {
	clusters, err := ListClusterAccounts(baseClient)
	if err != nil {
		return "", err
	}

	clusterNames := []string{}
	for _, cluster := range clusters {
		clusterNames = append(clusterNames, cluster.Cluster.Name)
	}

	if len(clusterNames) == 0 {
		return "", fmt.Errorf("the user has no access to any cluster")
	} else if len(clusterNames) == 1 {
		return clusterNames[0], nil
	}

	return log.Question(&survey.QuestionOptions{
		Question:     "Please choose a cluster to use",
		DefaultValue: clusterNames[0],
		Options:      clusterNames,
	})
}

// SelectAccount lets the user select an account in a cluster
func SelectAccount(baseClient client.Client, clusterName string, log log.Logger) (string, error) {
	clusters, err := ListClusterAccounts(baseClient)
	if err != nil {
		return "", err
	}

	accountNames := []string{}
	for _, cluster := range clusters {
		if cluster.Cluster.Name != clusterName {
			continue
		}

		accountNames = append(accountNames, cluster.Accounts...)
	}

	if len(accountNames) == 0 {
		return "", fmt.Errorf("the user has no account for cluster %s", clusterName)
	} else if len(accountNames) == 1 {
		return accountNames[0], nil
	}

	return log.Question(&survey.QuestionOptions{
		Question:     "Please choose a cluster to use",
		DefaultValue: accountNames[0],
		Options:      accountNames,
	})
}

func SelectPod(client kubernetes.Interface, namespace string, log log.Logger) (string, error) {
	podList, err := client.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app=virtualcluster",
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

// SelectSpaceAndClusterName selects a space and cluster name
func SelectSpaceAndClusterName(baseClient client.Client, spaceName, clusterName string, log log.Logger) (string, string, error) {
	client, err := baseClient.Management()
	if err != nil {
		return "", "", err
	}

	tokenInfo, err := baseClient.AuthInfo()
	if err != nil {
		return "", "", err
	}

	spaces, err := client.Loft().ManagementV1().Users().ListSpaces(context.TODO(), tokenInfo.Name, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}

	matchedSpaces := []managementv1.ClusterSpace{}
	questionOptions := []string{}
	for _, space := range spaces.Spaces {
		if spaceName != "" && space.Space.Name != spaceName {
			continue
		} else if clusterName != "" && space.Cluster != clusterName {
			continue
		}

		matchedSpaces = append(matchedSpaces, space)
		questionOptions = append(questionOptions, "Space: "+space.Space.Name+" - Cluster: "+space.Cluster)
	}

	if len(questionOptions) == 0 {
		if spaceName == "" {
			return "", "", fmt.Errorf("couldn't find any space")
		} else if clusterName != "" {
			return "", "", fmt.Errorf("couldn't find space %s in cluster %s", ansi.Color(spaceName, "white+b"), ansi.Color(clusterName, "white+b"))
		}

		return "", "", fmt.Errorf("couldn't find space %s", ansi.Color(spaceName, "white+b"))
	} else if len(questionOptions) == 1 {
		return spaceName, matchedSpaces[0].Cluster, nil
	}

	selectedSpace, err := log.Question(&survey.QuestionOptions{
		Question:     "Please choose a space to use",
		DefaultValue: questionOptions[0],
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

func SelectVirtualClusterAndSpaceAndClusterName(baseClient client.Client, virtualClusterName, spaceName, clusterName string, log log.Logger) (string, string, string, error) {
	client, err := baseClient.Management()
	if err != nil {
		return "", "", "", err
	}

	tokenInfo, err := baseClient.AuthInfo()
	if err != nil {
		return "", "", "", err
	}

	virtualClusters, err := client.Loft().ManagementV1().Users().ListVirtualClusters(context.TODO(), tokenInfo.Name, metav1.GetOptions{})
	if err != nil {
		return "", "", "", err
	}

	matchedVClusters := []managementv1.ClusterVirtualCluster{}
	questionOptions := []string{}
	for _, virtualCluster := range virtualClusters.VirtualClusters {
		if virtualClusterName != "" && virtualCluster.VirtualCluster.Name != virtualClusterName {
			continue
		} else if spaceName != "" && virtualCluster.VirtualCluster.Namespace != spaceName {
			continue
		} else if clusterName != "" && virtualCluster.Cluster != clusterName {
			continue
		}

		matchedVClusters = append(matchedVClusters, virtualCluster)
		questionOptions = append(questionOptions, "VirtualCluster: "+virtualCluster.VirtualCluster.Name+" - Space: "+virtualCluster.VirtualCluster.Namespace+" - Cluster: "+virtualCluster.Cluster)
	}

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
		DefaultValue: questionOptions[0],
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
