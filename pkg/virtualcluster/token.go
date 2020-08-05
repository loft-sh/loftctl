package virtualcluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"time"
)

func GetVirtualClusterToken(ctx context.Context, clusterClient kube.Interface, virtualClusterName, spaceName string) (string, error) {
	// wait until the secret exists
	var kubeConfigSecret *corev1.Secret
	virtualCluster, err := clusterClient.Loft().StorageV1().VirtualClusters(spaceName).Get(ctx, virtualClusterName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	secretName := "vc-" + virtualClusterName
	if virtualCluster.Spec.KubeConfigRef != nil && virtualCluster.Spec.KubeConfigRef.SecretName != "" {
		secretName = virtualCluster.Spec.KubeConfigRef.SecretName
	}

	secretKey := "config"
	if virtualCluster.Spec.KubeConfigRef != nil && virtualCluster.Spec.KubeConfigRef.Key != "" {
		secretKey = virtualCluster.Spec.KubeConfigRef.Key
	}

	err = wait.PollImmediate(time.Second, time.Minute*15, func() (bool, error) {
		secret, err := clusterClient.CoreV1().Secrets(spaceName).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) == false {
				return false, err
			}

			return false, nil
		}

		kubeConfigSecret = secret
		return true, nil
	})
	if err != nil {
		return "", err
	}

	kubeConfig, ok := kubeConfigSecret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("couldn't find kube config in virtual cluster secret")
	}

	config, err := clientcmd.Load(kubeConfig)
	if err != nil {
		return "", errors.Wrap(err, "load virtual cluster kube config")
	}

	token := ""
	for _, authInfo := range config.AuthInfos {
		if authInfo.ClientKeyData != nil && authInfo.ClientCertificateData != nil {
			token = base64.StdEncoding.EncodeToString(authInfo.ClientCertificateData) + ":" + base64.StdEncoding.EncodeToString(authInfo.ClientKeyData)
		}
	}
	if token == "" {
		return "", fmt.Errorf("couldn't update kube config, because it seems the virtual cluster kube config is invalid and missing client cert & client key")
	}

	return token, nil
}
