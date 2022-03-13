package vcluster

import (
	"context"
	"github.com/loft-sh/loftctl/v2/pkg/client"
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
	return wait.PollImmediate(time.Second, time.Minute*6, func() (bool, error) {
		_, err = vClusterClient.CoreV1().ServiceAccounts("default").Get(ctx, "default", metav1.GetOptions{})
		if err != nil && time.Now().After(nextMessage) {
			log.Warnf("Cannot reach virtual cluster because: %v. Loft will continue waiting, but this operation may timeout", err)
			nextMessage = time.Now().Add(waitDuration)
			return false, nil
		}

		return err == nil, nil
	})
}
