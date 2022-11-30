package vcluster

import (
	"context"
	managementv1 "github.com/loft-sh/api/v2/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/v2/pkg/client"
	"github.com/loft-sh/loftctl/v2/pkg/kube"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
)

var waitDuration = 20 * time.Second

func WaitForVCluster(ctx context.Context, baseClient client.Client, clusterName, spaceName, virtualClusterName string, log log.Logger) error {
	vClusterClient, err := baseClient.VirtualCluster(clusterName, spaceName, virtualClusterName)
	if err != nil {
		return err
	}

	now := time.Now()
	nextMessage := now.Add(waitDuration)

	warnCounter := 0

	return wait.PollImmediate(time.Second, time.Minute*6, func() (bool, error) {
		_, err = vClusterClient.CoreV1().ServiceAccounts("default").Get(ctx, "default", metav1.GetOptions{})
		if err != nil && time.Now().After(nextMessage) {
			if warnCounter > 1 {
				log.Warnf("Cannot reach virtual cluster because: %v. Loft will continue waiting, but this operation may timeout", err)
			} else {
				log.Info("Waiting for virtual cluster to be available...")
			}

			nextMessage = time.Now().Add(waitDuration)
			warnCounter++
			return false, nil
		}

		return err == nil, nil
	})
}

func WaitForVirtualClusterInstance(ctx context.Context, managementClient kube.Interface, namespace, name string, waitUntilReady bool, log log.Logger) (*managementv1.VirtualClusterInstance, error) {
	now := time.Now()
	nextMessage := now.Add(waitDuration)
	var virtualClusterInstance *managementv1.VirtualClusterInstance
	var err error
	if !waitUntilReady {
		virtualClusterInstance, err = managementClient.Loft().ManagementV1().VirtualClusterInstances(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return virtualClusterInstance, nil
	}

	warnCounter := 0

	return virtualClusterInstance, wait.PollImmediate(time.Second, time.Minute*6, func() (bool, error) {
		virtualClusterInstance, err = managementClient.Loft().ManagementV1().VirtualClusterInstances(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if virtualClusterInstance.Status.Phase != storagev1.InstanceReady && virtualClusterInstance.Status.Phase != storagev1.InstanceSleeping {
			if time.Now().After(nextMessage) {
				if warnCounter > 1 {
					log.Warnf("Cannot reach virtual cluster because: %s (%s). Loft will continue waiting, but this operation may timeout", virtualClusterInstance.Status.Message, virtualClusterInstance.Status.Reason)
				} else {
					log.Info("Waiting for virtual cluster to be available...")
				}
				nextMessage = time.Now().Add(waitDuration)
				warnCounter++
			}
			return false, nil
		}

		return true, nil
	})
}

func WaitForSpaceInstance(ctx context.Context, managementClient kube.Interface, namespace, name string, waitUntilReady bool, log log.Logger) (*managementv1.SpaceInstance, error) {
	now := time.Now()
	nextMessage := now.Add(waitDuration)
	var spaceInstance *managementv1.SpaceInstance
	var err error
	if !waitUntilReady {
		spaceInstance, err = managementClient.Loft().ManagementV1().SpaceInstances(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return spaceInstance, nil
	}

	warnCounter := 0

	return spaceInstance, wait.PollImmediate(time.Second, time.Minute*6, func() (bool, error) {
		spaceInstance, err = managementClient.Loft().ManagementV1().SpaceInstances(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if spaceInstance.Status.Phase != storagev1.InstanceReady && spaceInstance.Status.Phase != storagev1.InstanceSleeping {
			if time.Now().After(nextMessage) {
				if warnCounter > 1 {
					log.Warnf("Cannot reach space because: %s (%s). Loft will continue waiting, but this operation may timeout", spaceInstance.Status.Message, spaceInstance.Status.Reason)
				} else {
					log.Info("Waiting for space to be available...")
				}
				nextMessage = time.Now().Add(waitDuration)
				warnCounter++
			}
			return false, nil
		}

		return true, nil
	})
}
