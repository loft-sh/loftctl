package clihelper

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	clusterv1 "github.com/loft-sh/agentapi/v2/pkg/apis/loft/cluster/v1"
	storagev1 "github.com/loft-sh/api/v2/pkg/apis/storage/v1"

	jsonpatch "github.com/evanphx/json-patch"
	loftclientset "github.com/loft-sh/api/v2/pkg/client/clientset_generated/clientset"
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/loft-sh/loftctl/v2/pkg/portforward"
	"github.com/loft-sh/loftctl/v2/pkg/survey"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

const defaultReleaseName = "loft"

func GetDisplayName(name string, displayName string) string {
	if displayName != "" {
		return displayName
	}

	return name
}

func DisplayName(entityInfo *clusterv1.EntityInfo) string {
	if entityInfo == nil {
		return ""
	} else if entityInfo.DisplayName != "" {
		return entityInfo.DisplayName
	} else if entityInfo.Username != "" {
		return entityInfo.Username
	}

	return entityInfo.Name
}

func GetLoftIngressHost(kubeClient kubernetes.Interface, namespace string) (string, error) {
	ingress, err := kubeClient.NetworkingV1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
	if err != nil {
		ingress, err := kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
		if err != nil {
			return "", err
		} else {
			// find host
			for _, rule := range ingress.Spec.Rules {
				return rule.Host, nil
			}
		}
	} else {
		// find host
		for _, rule := range ingress.Spec.Rules {
			return rule.Host, nil
		}
	}

	return "", fmt.Errorf("couldn't find any host in loft ingress '%s/loft-ingress', please make sure you have not changed any deployed resources", namespace)
}

func WaitForReadyLoftAgentPod(kubeClient kubernetes.Interface, namespace string, log log.Logger) error {
	// wait until we have a running loft pod
	err := wait.Poll(time.Second*2, time.Minute*10, func() (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=loft-agent",
		})
		if err != nil {
			log.Warnf("Error trying to retrieve Loft Agent pod: %v", err)
			return false, nil
		} else if len(pods.Items) == 0 {
			return false, nil
		}

		sort.Slice(pods.Items, func(i, j int) bool {
			return pods.Items[i].CreationTimestamp.After(pods.Items[j].CreationTimestamp.Time)
		})

		loftPod := &pods.Items[0]
		found := false
		for _, containerStatus := range loftPod.Status.ContainerStatuses {
			if containerStatus.State.Running != nil && containerStatus.Ready {
				if containerStatus.Name == "agent" {
					found = true
				}

				continue
			}

			return false, nil
		}

		return found, nil
	})
	if err != nil {
		return err
	}

	return nil
}

func WaitForReadyLoftPod(kubeClient kubernetes.Interface, namespace string, log log.Logger) (*corev1.Pod, error) {
	// wait until we have a running loft pod
	now := time.Now()
	warningPrinted := false
	pod := &corev1.Pod{}
	err := wait.Poll(time.Second*2, time.Minute*10, func() (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=loft",
		})
		if err != nil {
			log.Warnf("Error trying to retrieve Loft pod: %v", err)
			return false, nil
		} else if len(pods.Items) == 0 {
			return false, nil
		}

		sort.Slice(pods.Items, func(i, j int) bool {
			return pods.Items[i].CreationTimestamp.After(pods.Items[j].CreationTimestamp.Time)
		})

		loftPod := &pods.Items[0]
		found := false
		for _, containerStatus := range loftPod.Status.ContainerStatuses {
			if containerStatus.State.Running != nil && containerStatus.Ready {
				if containerStatus.Name == "manager" {
					found = true
				}

				continue
			} else if containerStatus.State.Terminated != nil || (containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason == "CrashLoopBackOff") {
				out, err := kubeClient.CoreV1().Pods(namespace).GetLogs(loftPod.Name, &corev1.PodLogOptions{
					Container: "manager",
				}).Do(context.Background()).Raw()
				if err != nil {
					return false, fmt.Errorf("There seems to be an issue with loft starting up. Please reach out to our support at https://loft.sh/")
				}
				if strings.Contains(string(out), "register instance: Post \"https://license.loft.sh/register\": dial tcp") {
					return false, fmt.Errorf("Loft logs: \n%v \nThere seems to be an issue with Loft starting up. Looks like you try to install Loft into an air-gapped environment, please reach out to our support at https://loft.sh/ for an offline license and take a look at the air-gapped installation guide https://loft.sh/docs/guides/administration/air-gapped-installation", string(out))
				}

				return false, fmt.Errorf("Loft logs: \n%v \nThere seems to be an issue with loft starting up. Please reach out to our support at https://loft.sh/", string(out))
			} else if containerStatus.State.Waiting != nil && time.Now().After(now.Add(time.Minute*3)) && warningPrinted == false {
				log.Warnf("There might be an issue with Loft starting up. The container is still waiting, because of %s (%s). Please reach out to our support at https://loft.sh/", containerStatus.State.Waiting.Message, containerStatus.State.Waiting.Reason)
				warningPrinted = true
			}

			return false, nil
		}

		pod = loftPod
		return found, nil
	})
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func StartPortForwarding(config *rest.Config, client kubernetes.Interface, pod *corev1.Pod, localPort string, log log.Logger) (chan struct{}, error) {
	log.WriteString("\n")
	log.Info("Starting port-forwarding to the Loft pod")
	execRequest := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("portforward")

	t, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: t}, "POST", execRequest.URL())
	errChan := make(chan error)
	readyChan := make(chan struct{})
	stopChan := make(chan struct{})
	targetPort := getPortForwardingTargetPort(pod)
	forwarder, err := portforward.New(dialer, []string{localPort + ":" + strconv.Itoa(targetPort)}, stopChan, readyChan, errChan, ioutil.Discard, ioutil.Discard)
	if err != nil {
		return nil, err
	}

	go func() {
		err := forwarder.ForwardPorts()
		if err != nil {
			errChan <- err
		}
	}()

	// wait till ready
	select {
	case err = <-errChan:
		return nil, err
	case <-readyChan:
	case <-stopChan:
		return nil, fmt.Errorf("stopped before ready")
	}

	// start watcher
	go func() {
		for {
			select {
			case <-stopChan:
				return
			case err = <-errChan:
				log.Infof("error during port forwarder: %v", err)
				close(stopChan)
				return
			}
		}
	}()

	return stopChan, nil
}

func GetLoftDefaultPassword(kubeClient kubernetes.Interface, namespace string) (string, error) {
	loftNamespace, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			loftNamespace, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				return "", err
			}

			return string(loftNamespace.UID), nil
		}

		return "", err
	}

	return string(loftNamespace.UID), nil
}

type version struct {
	Version string `json:"version"`
}

func IsLoftReachable(host string) (bool, error) {
	// wait until loft is reachable at the given url
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	url := "https://" + host + "/version"
	resp, err := client.Get(url)
	if err == nil && resp.StatusCode == http.StatusOK {
		out, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}

		v := &version{}
		err = json.Unmarshal(out, v)
		if err != nil {
			return false, fmt.Errorf("error decoding response from %s: %v. Try running 'loft start --reset'", url, err)
		} else if v.Version == "" {
			return false, fmt.Errorf("unexpected response from %s: %s. Try running 'loft start --reset'", url, string(out))
		}

		return true, nil
	}

	return false, nil
}

func IsLocalCluster(host string, log log.Logger) bool {
	url, err := url.Parse(host)
	if err != nil {
		log.Warnf("Couldn't parse kube context host url: %v", err)
		return false
	}

	hostname := url.Hostname()
	ip := net.ParseIP(hostname)
	if ip != nil {
		if IsPrivateIP(ip) {
			return true
		}
	}

	if hostname == "localhost" || strings.HasSuffix(hostname, ".internal") || strings.HasSuffix(hostname, ".localhost") {
		return true
	}

	return false
}

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, _ := net.ParseCIDR(cidr)
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

// IsPrivateIP checks if a given ip is private
func IsPrivateIP(ip net.IP) bool {
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

func EnterHostNameQuestion(log log.Logger) (string, error) {
	return log.Question(&survey.QuestionOptions{
		Question: "Enter a hostname for your Loft instance (e.g. loft.my-domain.tld): \n ",
		ValidationFunc: func(answer string) error {
			u, err := url.Parse("https://" + answer)
			if err != nil || u.Path != "" || u.Port() != "" || len(strings.Split(answer, ".")) < 2 {
				return fmt.Errorf("please enter a valid hostname without protocol (https://), without path and without port, e.g. loft.my-domain.tld")
			}
			return nil
		},
	})
}

func IsLoftAlreadyInstalled(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	_, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), "loft", metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) == true {
			return false, nil
		}

		return false, fmt.Errorf("error accessing kubernetes cluster: %v", err)
	}

	return true, nil
}

func UninstallLoft(kubeClient kubernetes.Interface, restConfig *rest.Config, kubeContext, namespace string, log log.Logger) error {
	log.StartWait("Uninstalling loft...")
	defer log.StopWait()

	releaseName := defaultReleaseName
	deploy, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), "loft", metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	} else if deploy != nil && deploy.Labels != nil && deploy.Labels["release"] != "" {
		releaseName = deploy.Labels["release"]
	}

	args := []string{
		"uninstall",
		releaseName,
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	log.Infof("Executing command: helm %s", strings.Join(args, " "))
	output, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		log.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	// we also cleanup the validating webhook configuration and apiservice
	err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), "loft", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	apiRegistrationClient, err := clientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(context.TODO(), "v1.management.loft.sh", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	err = deleteUser(restConfig, "admin")
	if err != nil {
		return err
	}

	err = kubeClient.CoreV1().Secrets(namespace).Delete(context.Background(), "loft-user-secret-admin", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	// uninstall agent
	releaseName = "loft-agent"
	args = []string{
		"uninstall",
		releaseName,
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	log.WriteString("\n")
	log.Infof("Executing command: helm %s", strings.Join(args, " "))
	output, err = exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		log.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	// we also cleanup the validating webhook configuration and apiservice
	err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), "loft-agent", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(context.TODO(), "v1alpha1.tenancy.kiosk.sh", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(context.TODO(), "v1.cluster.loft.sh", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	err = kubeClient.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "loft-agent-controller", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	err = kubeClient.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "loft-applied-defaults", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	log.StopWait()
	log.WriteString("\n")
	log.Done("Successfully uninstalled Loft")
	log.WriteString("\n")

	return nil
}

func deleteUser(restConfig *rest.Config, name string) error {
	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	user, err := loftClient.StorageV1().Users().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil
	} else if len(user.Finalizers) > 0 {
		user.Finalizers = nil
		_, err = loftClient.StorageV1().Users().Update(context.TODO(), user, metav1.UpdateOptions{})
		if err != nil {
			if kerrors.IsConflict(err) {
				return deleteUser(restConfig, name)
			}

			return err
		}
	}

	err = loftClient.StorageV1().Users().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	return nil
}

func EnsureIngressController(kubeClient kubernetes.Interface, kubeContext string, log log.Logger) error {
	// first create an ingress controller
	const (
		YesOption = "Yes"
		NoOption  = "No, I already have an ingress controller installed."
	)

	answer, err := log.Question(&survey.QuestionOptions{
		Question:     "Ingress controller required. Should the nginx-ingress controller be installed?",
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
		args := []string{
			"install",
			"ingress-nginx",
			"ingress-nginx",
			"--repository-config=''",
			"--repo",
			"https://kubernetes.github.io/ingress-nginx",
			"--kube-context",
			kubeContext,
			"--namespace",
			"ingress-nginx",
			"--create-namespace",
			"--set-string",
			"controller.config.hsts=false",
			"--wait",
		}
		log.WriteString("\n")
		log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
		log.StartWait("Waiting for ingress controller deployment, this can take several minutes...")
		helmCmd := exec.Command("helm", args...)
		output, err := helmCmd.CombinedOutput()
		log.StopWait()
		if err != nil {
			return fmt.Errorf("error during helm command: %s (%v)", string(output), err)
		}

		list, err := kubeClient.CoreV1().Secrets("ingress-nginx").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "name=ingress-nginx,owner=helm,status=deployed",
		})
		if err != nil {
			return err
		}

		if len(list.Items) == 1 {
			secret := list.Items[0]
			originalSecret := secret.DeepCopy()
			secret.Labels["loft.sh/app"] = "true"
			if secret.Annotations == nil {
				secret.Annotations = map[string]string{}
			}

			secret.Annotations["loft.sh/url"] = "https://kubernetes.github.io/ingress-nginx"
			originalJSON, err := json.Marshal(originalSecret)
			if err != nil {
				return err
			}
			modifiedJSON, err := json.Marshal(secret)
			if err != nil {
				return err
			}
			data, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
			if err != nil {
				return err
			}
			_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Patch(context.TODO(), secret.Name, types.MergePatchType, data, metav1.PatchOptions{})
			if err != nil {
				return err
			}
		}

		log.Done("Successfully installed ingress-nginx to your kubernetes cluster!")
	}

	return nil
}

func UpgradeLoft(chartName, chartRepo, kubeContext, namespace string, extraArgs []string, log log.Logger) error {
	// now we install loft
	args := []string{
		"upgrade",
		defaultReleaseName,
		chartName,
		"--install",
		"--reuse-values",
		"--create-namespace",
		"--repository-config=''",
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	if chartRepo != "" {
		args = append(args, "--repo", chartRepo)
	}
	args = append(args, extraArgs...)

	log.WriteString("\n")
	log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
	log.StartWait("Waiting for helm command, this can take up to several minutes...")
	helmCmd := exec.Command("helm", args...)
	if chartRepo != "" {
		helmWorkDir, err := getHelmWorkdir(chartName)
		if err != nil {
			return err
		}

		helmCmd.Dir = helmWorkDir
	}
	output, err := helmCmd.CombinedOutput()
	log.StopWait()
	if err != nil {
		return fmt.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	log.Done("Loft has been deployed to your cluster!")
	return nil
}

func GetLoftManifests(chartName, chartRepo, kubeContext, namespace string, extraArgs []string, log log.Logger) (string, error) {
	args := []string{
		"template",
		defaultReleaseName,
		chartName,
		"--repository-config=''",
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	if chartRepo != "" {
		args = append(args, "--repo", chartRepo)
	}
	args = append(args, extraArgs...)

	helmCmd := exec.Command("helm", args...)
	if chartRepo != "" {
		helmWorkDir, err := getHelmWorkdir(chartName)
		if err != nil {
			return "", err
		}

		helmCmd.Dir = helmWorkDir
	}
	output, err := helmCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error during helm command: %s (%v)", string(output), err)
	}
	return string(output), nil
}

// Return the directory where the `helm` commands should be executed or error if none can be found/created
// Uses current workdir by default unless it contains a folder with the chart name
func getHelmWorkdir(chartName string) (string, error) {
	// If chartName folder exists, check temp dir next
	if _, err := os.Stat(chartName); err == nil {
		tempDir := os.TempDir()

		// If tempDir/chartName folder exists, create temp folder
		if _, err := os.Stat(path.Join(tempDir, chartName)); err == nil {
			tempDir, err = os.MkdirTemp(tempDir, chartName)
			if err != nil {
				return "", errors.New("problematic directory `" + chartName + "` found: please execute command in a different folder")
			}
		}

		// Use tempDir
		return tempDir, nil
	}

	// Use current workdir
	return "", nil
}

// Makes sure that admin user and password secret exists
// Returns (true, nil) if everything is correct but password is different from parameter `password`
func EnsureAdminPassword(kubeClient kubernetes.Interface, restConfig *rest.Config, password string, log log.Logger) (bool, error) {
	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return false, err
	}

	admin, err := loftClient.StorageV1().Users().Get(context.TODO(), "admin", metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return false, err
	} else if admin == nil {
		admin, err = loftClient.StorageV1().Users().Create(context.TODO(), &storagev1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin",
			},
			Spec: storagev1.UserSpec{
				Username: "admin",
				Email:    "test@domain.tld",
				Subject:  "admin",
				Groups:   []string{"system:masters"},
				PasswordRef: &storagev1.SecretRef{
					SecretName:      "loft-user-secret-admin",
					SecretNamespace: "loft",
					Key:             "password",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
	} else if admin.Spec.PasswordRef == nil || admin.Spec.PasswordRef.SecretName == "" || admin.Spec.PasswordRef.SecretNamespace == "" {
		return false, nil
	}

	key := admin.Spec.PasswordRef.Key
	if key == "" {
		key = "password"
	}

	passwordHash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))

	secret, err := kubeClient.CoreV1().Secrets(admin.Spec.PasswordRef.SecretNamespace).Get(context.TODO(), admin.Spec.PasswordRef.SecretName, metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return false, err
	} else if err == nil {
		existingPasswordHash, keyExists := secret.Data[key]
		if keyExists {
			return (string(existingPasswordHash) != passwordHash), nil
		}

		secret.Data[key] = []byte(passwordHash)
		_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
		if err != nil {
			return false, errors.Wrap(err, "update admin password secret")
		}
		return false, nil
	}

	// create the password secret if it was not found, this can happen if you delete the loft namespace without deleting the admin user
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      admin.Spec.PasswordRef.SecretName,
			Namespace: admin.Spec.PasswordRef.SecretNamespace,
		},
		Data: map[string][]byte{
			key: []byte(passwordHash),
		},
	}
	_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return false, errors.Wrap(err, "create admin password secret")
	}

	log.Info("Successfully recreated admin password secret")
	return false, nil
}

func IsLoftInstalledLocally(kubeClient kubernetes.Interface, namespace string) bool {
	_, err := kubeClient.NetworkingV1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		_, err = kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
		return kerrors.IsNotFound(err)
	}

	return kerrors.IsNotFound(err)
}

func getPortForwardingTargetPort(pod *corev1.Pod) int {
	for _, container := range pod.Spec.Containers {
		if container.Name == "manager" {
			for _, port := range container.Ports {
				if port.Name == "https" {
					return int(port.ContainerPort)
				}
			}
		}
	}

	return 443
}
