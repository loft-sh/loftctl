package vcluster

import (
	"context"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sync"
	"time"
)

var waitWarningAfter = time.Minute * 2

func WaitForVCluster(ctx context.Context, baseClient client.Client, clusterName, spaceName, virtualClusterName string, log log.Logger) error {
	vClusterClient, err := baseClient.VirtualCluster(clusterName, spaceName, virtualClusterName)
	if err != nil {
		return err
	}

	now := time.Now()
	warnOnce := sync.Once{}
	return wait.PollImmediate(time.Second, time.Minute*6, func() (bool, error) {
		_, err = vClusterClient.CoreV1().ServiceAccounts("default").Get(ctx, "default", metav1.GetOptions{})
		if err != nil && time.Since(now) > waitWarningAfter {
			warnOnce.Do(func() {
				log.Warnf("Cannot reach virtual cluster because: %v\n Will continue waiting, but this operation may timeout", err)
			})
			return false, nil
		}

		return err == nil, nil
	})
}
