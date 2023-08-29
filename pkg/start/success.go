package start

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/loft-sh/loftctl/v3/pkg/clihelper"
	"github.com/loft-sh/loftctl/v3/pkg/config"
	"github.com/loft-sh/loftctl/v3/pkg/printhelper"
	"github.com/loft-sh/log/survey"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (l *LoftStarter) success(ctx context.Context) error {
	if l.NoWait {
		return nil
	}

	// wait until Loft is ready
	loftPod, err := l.waitForLoft()
	if err != nil {
		return err
	}

	if l.NoPortForwarding {
		return nil
	}

	// check if Loft was installed locally
	isLocal := clihelper.IsLoftInstalledLocally(l.KubeClient, l.Namespace)
	if isLocal {
		// check if loft domain secret is there
		if !l.NoTunnel {
			loftRouterDomain, err := l.pingLoftRouter(ctx, loftPod)
			if err != nil {
				l.Log.Errorf("Error retrieving loft router domain: %v", err)
				l.Log.Info("Fallback to use port-forwarding")
			} else if loftRouterDomain != "" {
				printhelper.PrintSuccessMessageLoftRouterInstall(loftRouterDomain, l.Password, l.Log)
				return nil
			}
		}

		// start port-forwarding
		err = l.startPortForwarding(ctx, loftPod)
		if err != nil {
			return err
		}

		return l.successLocal()
	}

	// get login link
	l.Log.Info("Checking Loft status...")
	host, err := clihelper.GetLoftIngressHost(l.KubeClient, l.Namespace)
	if err != nil {
		return err
	}

	// check if loft is reachable
	reachable, err := clihelper.IsLoftReachable(host)
	if !reachable || err != nil {
		const (
			YesOption = "Yes"
			NoOption  = "No, please re-run the DNS check"
		)

		answer, err := l.Log.Question(&survey.QuestionOptions{
			Question:     "Unable to reach Loft at https://" + host + ". Do you want to start port-forwarding instead?",
			DefaultValue: YesOption,
			Options: []string{
				YesOption,
				NoOption,
			},
		})
		if err != nil {
			return err
		}

		if answer == YesOption {
			err = l.startPortForwarding(ctx, loftPod)
			if err != nil {
				return err
			}

			return l.successLocal()
		}
	}

	return l.successRemote(host)
}

func (l *LoftStarter) pingLoftRouter(ctx context.Context, loftPod *corev1.Pod) (string, error) {
	loftRouterSecret, err := l.KubeClient.CoreV1().Secrets(loftPod.Namespace).Get(ctx, clihelper.LoftRouterDomainSecret, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("find loft router domain secret: %w", err)
	} else if loftRouterSecret.Data == nil || len(loftRouterSecret.Data["domain"]) == 0 {
		return "", nil
	}

	// get the domain from secret
	loftRouterDomain := string(loftRouterSecret.Data["domain"])

	// wait until loft is reachable at the given url
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	l.Log.Infof("Waiting until loft is reachable at https://%s", loftRouterDomain)
	err = wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*5, true, func(ctx context.Context) (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+loftRouterDomain+"/version", nil)
		if err != nil {
			return false, nil
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return false, nil
		}

		return resp.StatusCode == http.StatusOK, nil
	})
	if err != nil {
		return "", err
	}

	return loftRouterDomain, nil
}

func (l *LoftStarter) successLocal() error {
	printhelper.PrintSuccessMessageLocalInstall(l.Password, l.LocalPort, l.Log)

	blockChan := make(chan bool)
	<-blockChan
	return nil
}

func (l *LoftStarter) successRemote(host string) error {
	ready, err := clihelper.IsLoftReachable(host)
	if err != nil {
		return err
	} else if ready {
		printhelper.PrintSuccessMessageRemoteInstall(host, l.Password, l.Log)
		return nil
	}

	// Print DNS Configuration
	printhelper.PrintDNSConfiguration(host, l.Log)

	l.Log.Info("Waiting for you to configure DNS, so loft can be reached on https://" + host)
	err = wait.PollImmediate(time.Second*5, config.Timeout(), func() (bool, error) {
		return clihelper.IsLoftReachable(host)
	})
	if err != nil {
		return err
	}

	l.Log.Done("Loft is reachable at https://" + host)
	printhelper.PrintSuccessMessageRemoteInstall(host, l.Password, l.Log)
	return nil
}

func (l *LoftStarter) waitForLoft() (*corev1.Pod, error) {
	// wait for loft pod to start
	l.Log.Info("Waiting for Loft pod to be running...")
	loftPod, err := clihelper.WaitForReadyLoftPod(l.KubeClient, l.Namespace, l.Log)
	l.Log.Donef("Loft pod successfully started")
	if err != nil {
		return nil, err
	}

	// ensure user admin secret is there
	isNewPassword, err := clihelper.EnsureAdminPassword(l.KubeClient, l.RestConfig, l.Password, l.Log)
	if err != nil {
		return nil, err
	}

	// If password is different than expected
	if isNewPassword {
		l.Password = ""
	}

	return loftPod, nil
}
