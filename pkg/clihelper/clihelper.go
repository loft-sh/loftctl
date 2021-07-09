package clihelper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	loftclientset "github.com/loft-sh/api/pkg/client/clientset_generated/clientset"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/pkg/errors"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

func GetLoftIngressHost(kubeClient kubernetes.Interface, namespace string) (string, error) {
	ingress, err := kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// find host
	for _, rule := range ingress.Spec.Rules {
		return rule.Host, nil
	}
	return "", fmt.Errorf("couldn't find any host in loft ingress '%s/loft-ingress', please make sure you have not changed any deployed resources")
}

func WaitForReadyLoftPod(kubeClient kubernetes.Interface, namespace string, log log.Logger) error {
	// wait until we have a running loft pod
	now := time.Now()
	warningPrinted := false
	return wait.PollImmediate(time.Second*2, time.Minute*10, func() (bool, error) {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=loft",
		})
		if err != nil {
			log.Warnf("Error trying to retrieve loft pod: %v", err)
			return false, nil
		} else if len(pods.Items) == 0 {
			return false, nil
		}

		loftPod := &pods.Items[0]
		found := false
		for _, containerStatus := range loftPod.Status.ContainerStatuses {
			if containerStatus.State.Running != nil && containerStatus.Ready {
				if containerStatus.Name == "manager" {
					found = true
				}

				continue
			} else if containerStatus.State.Terminated != nil {
				out, err := kubeClient.CoreV1().Pods(namespace).GetLogs(loftPod.Name, &corev1.PodLogOptions{
					Container: "manager",
				}).Do(context.Background()).Raw()
				if err != nil {
					return false, fmt.Errorf("There seems to be an issue with loft starting up. Please reach out to our support at https://loft.sh/")
				}
				if strings.Contains(string(out), "register instance: Post \"https://license.loft.sh/register\": dial tcp") {
					return false, fmt.Errorf("There seems to be an issue with loft starting up. Looks like you try to install Loft into an air-gapped environment, please reach out to our support at https://loft.sh/ for an offline license. Loft logs: \n%v", string(out))
				}

				return false, fmt.Errorf("There seems to be an issue with loft starting up. Please reach out to our support at https://loft.sh/. Loft logs: \n%v", string(out))
			} else if containerStatus.State.Waiting != nil && time.Now().After(now.Add(time.Minute*3)) && warningPrinted == false {
				log.Warnf("There might be an issue with loft starting up. The container is still waiting, because of %s (%s). Please reach out to our support at https://loft.sh/", containerStatus.State.Waiting.Message, containerStatus.State.Waiting.Reason)
				warningPrinted = true
			}

			return false, nil
		}

		return found, nil
	})
}

func StartPortForwarding(kubeClient kubernetes.Interface, kubeContext, namespace string, localPort string, log log.Logger) error {
	log.WriteString("\n")
	log.Info("Loft will now start port-forwarding to the loft pod")
	args := []string{
		"port-forward",
		"deploy/loft",
		"--context",
		kubeContext,
		"--namespace",
		namespace,
		localPort + ":443",
	}
	log.Infof("Starting command: kubectl %s", strings.Join(args, " "))

	buffer := &bytes.Buffer{}
	c := exec.Command("kubectl", args...)
	c.Stderr = buffer
	// c.Stdout = os.Stdout

	err := c.Start()
	if err != nil {
		return fmt.Errorf("error starting kubectl command: %v", err)
	}
	go func() {
		err := c.Wait()
		if err != nil {
			log.Fatalf("Port-Forwarding has unexpectedly ended. Please restart the command via 'loft start'. Error: %s", buffer.String())
		}
	}()

	// wait until loft is reachable at the given url
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	log.Infof("Waiting until loft is reachable at https://localhost:%s", localPort)
	return wait.PollImmediate(time.Second, time.Minute*10, func() (bool, error) {
		resp, err := client.Get("https://localhost:" + localPort + "/version")
		if err != nil {
			return false, nil
		}

		return resp.StatusCode == http.StatusOK, nil
	})
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

func AskForHost(log log.Logger) (string, error) {
	ingressAccess := "via ingress (you will need to configure DNS)"
	answer, err := log.Question(&survey.QuestionOptions{
		Question: "How do you want to access loft?",
		Options: []string{
			"via port-forwarding (no other configuration needed)",
			ingressAccess,
		},
	})
	if err != nil {
		return "", err
	}

	if answer == ingressAccess {
		return EnterHostNameQuestion(log)
	}

	return "", nil
}

func EnterHostNameQuestion(log log.Logger) (string, error) {
	return log.Question(&survey.QuestionOptions{
		Question: "Enter a hostname for your loft instance (e.g. loft.my-domain.tld): \n",
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

	deploy, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), "loft", metav1.GetOptions{})
	if err != nil {
		return err
	} else if deploy.Labels == nil || deploy.Labels["release"] == "" {
		return fmt.Errorf("loft was not installed via helm, cannot delete it then")
	}

	releaseName := deploy.Labels["release"]
	args := []string{
		"uninstall",
		releaseName,
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	log.WriteString("\n")
	log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
	output, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	// wait for the loft pods to terminate
	err = wait.Poll(time.Second, time.Minute*10, func() (bool, error) {
		list, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=loft"})
		if err != nil {
			return false, err
		}

		return len(list.Items) == 0, nil
	})
	if err != nil {
		return err
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

	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	err = loftClient.StorageV1().Users().Delete(context.TODO(), "admin", metav1.DeleteOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	}

	log.StopWait()
	log.Done("Successfully uninstalled loft")
	return nil
}

func InstallIngressController(kubeClient kubernetes.Interface, kubeContext string, log log.Logger) error {
	// first create an ingress controller
	const (
		YesOption = "Yes"
		NoOption  = "No, I already have an ingress controller installed"
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
		output, err := exec.Command("helm", args...).CombinedOutput()
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

func UpgradeLoft(kubeContext, namespace string, extraArgs []string, log log.Logger) error {
	// now we install loft
	args := []string{
		"upgrade",
		"loft",
		"loft",
		"--install",
		"--create-namespace",
		"--repository-config=''",
		"--repo",
		"https://charts.devspace.sh/",
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	args = append(args, extraArgs...)

	log.WriteString("\n")
	log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
	log.StartWait("Waiting for helm command, this can take up to several minutes...")
	output, err := exec.Command("helm", args...).CombinedOutput()
	log.StopWait()
	if err != nil {
		return fmt.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	log.Done("Successfully deployed loft to your kubernetes cluster!")
	log.WriteString("\n")
	return nil
}

func defaultHelmValues(password, email, version, values string, extraArgs []string) []string {
	// now we install loft
	args := []string{
		"--set",
		"certIssuer.create=false",
		"--set",
		"cluster.connect.local=true",
		"--set",
		"admin.password=" + password,
		"--set",
		"admin.email=" + email,
	}
	if version != "" {
		args = append(args, "--version", version)
	}
	if values != "" {
		args = append(args, "--values", values)
	}
	args = append(args, extraArgs...)
	return args
}

func InstallLoftRemote(kubeContext, namespace, password, email, version, values, host string, log log.Logger) error {
	extraArgs := defaultHelmValues(password, email, version, values, []string{
		"--set",
		"ingress.enabled=true",
		"--set",
		"ingress.host=" + host,
	})

	return UpgradeLoft(kubeContext, namespace, extraArgs, log)
}

func InstallLoftLocally(kubeContext, namespace, password, email, version, values string, log log.Logger) error {
	log.WriteString("\n")
	log.Info("This will install loft without an externally reachable URL and instead use port-forwarding to connect to loft")
	log.WriteString("\n")

	// deploy loft into the cluster
	extraArgs := defaultHelmValues(password, email, version, values, []string{
		"--set",
		"ingress.enabled=false",
	})

	return UpgradeLoft(kubeContext, namespace, extraArgs, log)
}

func EnsureAdminPassword(kubeClient kubernetes.Interface, restConfig *rest.Config, password string, log log.Logger) error {
	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	admin, err := loftClient.StorageV1().Users().Get(context.TODO(), "admin", metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	} else if admin == nil || admin.Spec.PasswordRef == nil || admin.Spec.PasswordRef.SecretName == "" || admin.Spec.PasswordRef.SecretNamespace == "" {
		return nil
	}

	_, err = kubeClient.CoreV1().Secrets(admin.Spec.PasswordRef.SecretNamespace).Get(context.TODO(), admin.Spec.PasswordRef.SecretName, metav1.GetOptions{})
	if err != nil && kerrors.IsNotFound(err) == false {
		return err
	} else if err == nil {
		return nil
	}

	key := admin.Spec.PasswordRef.Key
	if key == "" {
		key = "password"
	}

	// create the password secret if it was not found, this can happen if you delete the loft namespace without deleting the admin user
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      admin.Spec.PasswordRef.SecretName,
			Namespace: admin.Spec.PasswordRef.SecretNamespace,
		},
		Data: map[string][]byte{
			key: []byte(fmt.Sprintf("%x", sha256.Sum256([]byte(password)))),
		},
	}
	_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create admin password secret")
	}

	log.Info("Successfully recreated admin password secret")
	return nil
}

func IsLoftInstalledLocally(kubeClient kubernetes.Interface, namespace string) bool {
	_, err := kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(context.TODO(), "loft-ingress", metav1.GetOptions{})
	return kerrors.IsNotFound(err)
}
