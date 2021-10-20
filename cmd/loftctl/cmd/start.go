package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	loftclient "github.com/loft-sh/api/pkg/client/clientset_generated/clientset"
	"github.com/loft-sh/loftctl/pkg/clihelper"
	"github.com/loft-sh/loftctl/pkg/printhelper"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"net/http"
	"os/exec"
	"regexp"
	"time"

	"github.com/loft-sh/loftctl/cmd/loftctl/flags"
	"github.com/loft-sh/loftctl/pkg/client"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var emailRegex = regexp.MustCompile("^[^@]+@[^\\.]+\\..+$")

// StartCmd holds the cmd flags
type StartCmd struct {
	*flags.GlobalFlags

	LocalPort   string
	Reset       bool
	Version     string
	Context     string
	Namespace   string
	Password    string
	Values      string
	ReuseValues bool
	Upgrade     bool

	// Will be filled later
	KubeClient kubernetes.Interface
	RestConfig *rest.Config
	Log        log.Logger
}

// NewStartCmd creates a new command
func NewStartCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &StartCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start a loft instance and connect via port-forwarding",
		Long: `
#######################################################
###################### loft start #####################
#######################################################
Starts a loft instance in your Kubernetes cluster and
then establishes a port-forwarding connection.

Please make sure you meet the following requirements 
before running this command:

1. Current kube-context has admin access to the cluster
2. Helm v3 must be installed
3. kubectl must be installed

#######################################################
	`,
		Args: cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()

			return cmd.Run(cobraCmd, args)
		},
	}

	startCmd.Flags().StringVar(&cmd.Context, "context", "", "The kube context to use for installation")
	startCmd.Flags().StringVar(&cmd.Namespace, "namespace", "loft", "The namespace to install loft into")
	startCmd.Flags().StringVar(&cmd.LocalPort, "local-port", "9898", "The local port to bind to if using port-forwarding")
	startCmd.Flags().StringVar(&cmd.Password, "password", "", "The password to use for the admin account. (If empty this will be the namespace UID)")
	startCmd.Flags().StringVar(&cmd.Version, "version", "", "The loft version to install")
	startCmd.Flags().StringVar(&cmd.Values, "values", "", "Path to a file for extra loft helm chart values")
	startCmd.Flags().BoolVar(&cmd.ReuseValues, "reuse-values", true, "Reuse previous Loft helm values on upgrade")
	startCmd.Flags().BoolVar(&cmd.Upgrade, "upgrade", false, "If true, Loft will try to upgrade the release")
	startCmd.Flags().BoolVar(&cmd.Reset, "reset", false, "If true, an existing loft instance will be deleted before installing loft")
	return startCmd
}

// Run executes the functionality "loft start"
func (cmd *StartCmd) Run(cobraCmd *cobra.Command, args []string) error {
	err := cmd.prepare()
	if err != nil {
		return err
	}

	// Is already installed?
	isInstalled, err := clihelper.IsLoftAlreadyInstalled(cmd.KubeClient, cmd.Namespace)
	if err != nil {
		return err
	} else if isInstalled {
		if cmd.Reset == false {
			return cmd.handleAlreadyExistingInstallation()
		}

		cmd.Log.Info("Found an existing loft installation")
		err = clihelper.UninstallLoft(cmd.KubeClient, cmd.RestConfig, cmd.Context, cmd.Namespace, cmd.Log)
		if err != nil {
			return err
		}
	}

	cmd.Log.WriteString("\n")
	cmd.Log.Info("Welcome to the loft installation.")
	cmd.Log.Info("This installer will guide you through the installation.")
	cmd.Log.Info("If you prefer installing loft via helm yourself, visit https://loft.sh/docs/getting-started/setup")
	cmd.Log.Info("Thanks for trying out loft!")

	installLocally := clihelper.IsLocalCluster(cmd.RestConfig.Host, cmd.Log)
	remoteHost := ""

	if installLocally == false {
		const (
			YesOption = "Yes"
			NoOption  = "No, my cluster is running locally (docker desktop, minikube, kind etc.)"
		)

		answer, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Seems like your cluster is running remotely (GKE, EKS, AKS, private cloud etc.). Is that correct?",
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
			remoteHost, err = clihelper.AskForHost(cmd.Log)
			if err != nil {
				return err
			} else if remoteHost == "" {
				installLocally = true
			}
		} else {
			installLocally = true
		}
	} else {
		const (
			YesOption = "Yes"
			NoOption  = "No, I am using a remote cluster and want to access loft on a public domain"
		)

		answer, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Seems like your cluster is running locally (docker desktop, minikube, kind etc.). Is that correct?",
			DefaultValue: YesOption,
			Options: []string{
				YesOption,
				NoOption,
			},
		})
		if err != nil {
			return err
		}

		if answer == NoOption {
			installLocally = false

			remoteHost, err = clihelper.AskForHost(cmd.Log)
			if err != nil {
				return err
			} else if remoteHost == "" {
				installLocally = true
			}
		}
	}

	userEmail, err := cmd.Log.Question(&survey.QuestionOptions{
		Question: "Enter an email address for your admin user",
		ValidationFunc: func(emailVal string) error {
			if !emailRegex.MatchString(emailVal) {
				return fmt.Errorf("%s is not a valid email address", emailVal)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	
	// make sure we are ready for installing
	err = cmd.prepareInstall()
	if err != nil {
		return err
	}

	if installLocally || remoteHost == "" {
		return cmd.installLocal(userEmail)
	}

	return cmd.installRemote(userEmail, remoteHost)
}

func (cmd *StartCmd) prepareInstall() error {
	// delete admin user & secret
	loftClient, err := loftclient.NewForConfig(cmd.RestConfig)
	if err != nil {
		return err
	}
	
	_ = loftClient.StorageV1().Users().Delete(context.Background(), "admin", metav1.DeleteOptions{})
	_ = cmd.KubeClient.CoreV1().Secrets(cmd.Namespace).Delete(context.Background(), "loft-user-secret-admin", metav1.DeleteOptions{})
	return nil
}

func (cmd *StartCmd) prepare() error {
	loader, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}
	loftConfig := loader.Config()

	// first load the kube config
	kubeClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})

	// load the raw config
	kubeConfig, err := kubeClientConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	// we switch the context to the install config
	contextToLoad := kubeConfig.CurrentContext
	if cmd.Context != "" {
		contextToLoad = cmd.Context
	} else if loftConfig.LastInstallContext != "" && loftConfig.LastInstallContext != contextToLoad {
		contextToLoad, err = cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Seems like you tried to use 'loft start' with a different kubernetes context than before. Please choose which kubernetes context you want to use",
			DefaultValue: contextToLoad,
			Options:      []string{contextToLoad, loftConfig.LastInstallContext},
		})
		if err != nil {
			return err
		}
	}
	cmd.Context = contextToLoad

	loftConfig.LastInstallContext = contextToLoad
	_ = loader.Save()

	// kube client config
	kubeClientConfig = clientcmd.NewNonInteractiveClientConfig(kubeConfig, contextToLoad, &clientcmd.ConfigOverrides{}, clientcmd.NewDefaultClientConfigLoadingRules())

	// test for helm and kubectl
	_, err = exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("seems like helm is not installed. Helm is required for the installation of loft. Please visit https://helm.sh/docs/intro/install/ for install instructions")
	}

	output, err := exec.Command("helm", "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("seems like there are issues with your helm client: \n\n%s", output)
	}

	_, err = exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("seems like kubectl is not installed. Kubectl is required for the installation of loft. Please visit https://kubernetes.io/docs/tasks/tools/install-kubectl/ for install instructions")
	}

	output, err = exec.Command("kubectl", "version", "--context", contextToLoad).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Seems like kubectl cannot connect to your Kubernetes cluster: \n\n%s", output)
	}

	cmd.RestConfig, err = kubeClientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}
	cmd.KubeClient, err = kubernetes.NewForConfig(cmd.RestConfig)
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	// Check if cluster has RBAC correctly configured
	_, err = cmd.KubeClient.RbacV1().ClusterRoles().Get(context.Background(), "cluster-admin", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error retrieving cluster role 'cluster-admin': %v. Please make sure RBAC is correctly configured in your cluster", err)
	}

	return nil
}

func (cmd *StartCmd) handleAlreadyExistingInstallation() error {
	cmd.Log.Info("Found an existing loft installation, if you want to reinstall loft run 'loft start --reset'")
	cmd.Log.Info("Found an existing loft installation, if you want to upgrade loft run 'loft start --upgrade'")

	// get default password
	password := cmd.Password
	if password == "" {
		defaultPassword, err := clihelper.GetLoftDefaultPassword(cmd.KubeClient, cmd.Namespace)
		if err != nil {
			return err
		}

		password = defaultPassword
	}

	// check if we should upgrade Loft
	isLocal := clihelper.IsLoftInstalledLocally(cmd.KubeClient, cmd.Namespace)
	if cmd.Upgrade {
		extraArgs := []string{}
		if cmd.ReuseValues {
			extraArgs = append(extraArgs, "--reuse-values")
		}
		if cmd.Version != "" {
			extraArgs = append(extraArgs, "--version", cmd.Version)
		}
		if cmd.Values != "" {
			extraArgs = append(extraArgs, "--values", cmd.Values)
		}

		err := clihelper.UpgradeLoft(cmd.Context, cmd.Namespace, extraArgs, cmd.Log)
		if err != nil {
			return errors.Wrap(err, "upgrade loft")
		}
	} else if isLocal {
		// ask if we should deploy an ingress now
		const (
			NoOption  = "No"
			YesOption = "Yes, I want to deploy an ingress to let other people access loft."
		)

		answer, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Loft was installed without an ingress. Do you want to upgrade loft and install an ingress now?",
			DefaultValue: NoOption,
			Options: []string{
				NoOption,
				YesOption,
			},
		})
		if err != nil {
			return err
		} else if answer == YesOption {
			host, err := clihelper.EnterHostNameQuestion(cmd.Log)
			if err != nil {
				return err
			}

			err = cmd.upgradeWithIngress(host)
			if err != nil {
				return err
			}
		}
	}

	// recheck if Loft was installed locally
	isLocal = clihelper.IsLoftInstalledLocally(cmd.KubeClient, cmd.Namespace)

	// wait until Loft is ready
	loftPod, err := cmd.waitForLoft(password)
	if err != nil {
		return err
	}

	// check if local or remote installation
	if isLocal {
		err = cmd.startPortForwarding(loftPod)
		if err != nil {
			return err
		}

		return cmd.successLocal(password)
	}

	// get login link
	cmd.Log.StartWait("Checking loft status...")
	host, err := clihelper.GetLoftIngressHost(cmd.KubeClient, cmd.Namespace)
	cmd.Log.StopWait()
	if err != nil {
		return err
	}

	// check if loft is reachable
	reachable, err := clihelper.IsLoftReachable(host)
	if reachable == false || err != nil {
		const (
			YesOption = "Yes"
			NoOption  = "No, I want to see the DNS message again"
		)

		answer, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Loft seems to be not reachable at https://" + host + ". Do you want to use port-forwarding instead?",
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
			err = cmd.startPortForwarding(loftPod)
			if err != nil {
				return err
			}

			return cmd.successLocal(password)
		}
	}

	return cmd.successRemote(host, password)
}

func (cmd *StartCmd) waitForLoft(password string) (*corev1.Pod, error) {
	// wait for loft pod to start
	cmd.Log.StartWait("Waiting until loft pod has been started...")
	loftPod, err := clihelper.WaitForReadyLoftPod(cmd.KubeClient, cmd.Namespace, cmd.Log)
	cmd.Log.StopWait()
	cmd.Log.Donef("Loft pod successfully started")
	if err != nil {
		return nil, err
	}

	// wait for loft pod to start
	cmd.Log.StartWait("Waiting until loft agent has been started...")
	err = clihelper.WaitForReadyLoftAgentPod(cmd.KubeClient, cmd.Namespace, cmd.Log)
	cmd.Log.StopWait()
	if err != nil {
		return nil, err
	}

	// ensure user admin secret is there
	err = clihelper.EnsureAdminPassword(cmd.KubeClient, cmd.RestConfig, password, cmd.Log)
	if err != nil {
		return nil, err
	}

	return loftPod, nil
}

func (cmd *StartCmd) installRemote(email, host string) error {
	err := clihelper.InstallIngressController(cmd.KubeClient, cmd.Context, cmd.Log)
	if err != nil {
		return errors.Wrap(err, "install ingress controller")
	}

	password := cmd.Password
	if password == "" {
		defaultPassword, err := clihelper.GetLoftDefaultPassword(cmd.KubeClient, cmd.Namespace)
		if err != nil {
			return err
		}

		password = defaultPassword
	}

	err = clihelper.InstallLoftRemote(cmd.Context, cmd.Namespace, password, email, cmd.Version, cmd.Values, host, cmd.Log)
	if err != nil {
		return err
	}

	// wait until Loft is ready
	_, err = cmd.waitForLoft(password)
	if err != nil {
		return err
	}

	cmd.Log.Done("Loft pod has successfully started")
	return cmd.successRemote(host, password)
}

func (cmd *StartCmd) upgradeWithIngress(host string) error {
	err := clihelper.InstallIngressController(cmd.KubeClient, cmd.Context, cmd.Log)
	if err != nil {
		return errors.Wrap(err, "install ingress controller")
	}

	extraArgs := []string{
		"--reuse-values",
		"--set",
		"ingress.enabled=true",
		"--set",
		"ingress.host=" + host,
	}

	// upgrade loft
	err = clihelper.UpgradeLoft(cmd.Context, cmd.Namespace, extraArgs, cmd.Log)
	if err != nil {
		return err
	}

	return nil
}

func (cmd *StartCmd) installLocal(email string) error {
	password := cmd.Password
	if password == "" {
		defaultPassword, err := clihelper.GetLoftDefaultPassword(cmd.KubeClient, cmd.Namespace)
		if err != nil {
			return err
		}

		password = defaultPassword
	}

	err := clihelper.InstallLoftLocally(cmd.Context, cmd.Namespace, password, email, cmd.Version, cmd.Values, cmd.Log)
	if err != nil {
		return err
	}

	// wait until Loft is ready
	loftPod, err := cmd.waitForLoft(password)
	if err != nil {
		return err
	}

	err = cmd.startPortForwarding(loftPod)
	if err != nil {
		return err
	}
	
	return cmd.successLocal(password)
}

func (cmd *StartCmd) startPortForwarding(loftPod *corev1.Pod) error {
	stopChan, err := clihelper.StartPortForwarding(cmd.RestConfig, cmd.KubeClient, loftPod, cmd.LocalPort, cmd.Log)
	if err != nil {
		return err
	}
	go cmd.restartPortForwarding(stopChan)

	// wait until loft is reachable at the given url
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	cmd.Log.Infof("Waiting until loft is reachable at https://localhost:%s", cmd.LocalPort)
	err = wait.PollImmediate(time.Second, time.Minute*10, func() (bool, error) {
		resp, err := httpClient.Get("https://localhost:" + cmd.LocalPort + "/version")
		if err != nil {
			return false, nil
		}

		return resp.StatusCode == http.StatusOK, nil
	})
	if err != nil {
		return err
	}
	
	return nil 
}

func (cmd *StartCmd) restartPortForwarding(stopChan chan struct{}) {
	for {
		<- stopChan
		cmd.Log.Info("Restart port forwarding")

		// wait for loft pod to start
		cmd.Log.StartWait("Waiting until loft pod has been started...")
		loftPod, err := clihelper.WaitForReadyLoftPod(cmd.KubeClient, cmd.Namespace, cmd.Log)
		cmd.Log.StopWait()
		if err != nil {
			cmd.Log.Fatalf("Error waiting for ready loft pod: %v", err)
		}

		// restart port forwarding
		stopChan, err = clihelper.StartPortForwarding(cmd.RestConfig, cmd.KubeClient, loftPod, cmd.LocalPort, cmd.Log)
		if err != nil {
			cmd.Log.Fatalf("Error starting port forwarding: %v", err)
		}
		
		cmd.Log.Donef("Successfully restarted port forwarding")
	}
}

func (cmd *StartCmd) successRemote(host string, password string) error {
	ready, err := clihelper.IsLoftReachable(host)
	if err != nil {
		return err
	} else if ready {
		printhelper.PrintSuccessMessageRemoteInstall(host, password, cmd.Log)
		return nil
	}

	// Print DNS Configuration
	printhelper.PrintDNSConfiguration(host, cmd.Log)

	cmd.Log.StartWait("Waiting for you to configure DNS, so loft can be reached on https://" + host)
	err = wait.PollImmediate(time.Second*5, time.Hour*24, func() (bool, error) {
		return clihelper.IsLoftReachable(host)
	})
	cmd.Log.StopWait()
	if err != nil {
		return err
	}

	cmd.Log.Done("loft is reachable at https://" + host)
	printhelper.PrintSuccessMessageRemoteInstall(host, password, cmd.Log)
	return nil
}

func (cmd *StartCmd) successLocal(password string) error {
	printhelper.PrintSuccessMessageLocalInstall(password, cmd.LocalPort, cmd.Log)

	blockChan := make(chan bool)
	<-blockChan
	return nil
}
