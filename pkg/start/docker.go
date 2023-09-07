package start

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/denisbrodbeck/machineid"
	"github.com/loft-sh/api/v3/pkg/product"
	"github.com/loft-sh/loftctl/v3/pkg/clihelper"
	"github.com/loft-sh/log"
	"github.com/loft-sh/log/hash"
	"github.com/loft-sh/log/scanner"
	"github.com/mgutz/ansi"
	"github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	ErrMissingContainer = errors.New(product.Replace("couldn't find Loft container after starting it"))
	ErrLoftNotReachable = errors.New(product.Replace("cannot connect to Loft as it has no exposed port and --no-tunnel is enabled"))
)

type ContainerDetails struct {
	ID              string                   `json:"ID,omitempty"`
	Created         string                   `json:"Created,omitempty"`
	State           ContainerDetailsState    `json:"State,omitempty"`
	Config          ContainerDetailsConfig   `json:"Config,omitempty"`
	NetworkSettings ContainerNetworkSettings `json:"NetworkSettings,omitempty"`
}

type ContainerNetworkSettings struct {
	Ports map[string][]ContainerPort `json:"ports,omitempty"`
}

type ContainerPort struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

type ContainerDetailsConfig struct {
	Image  string            `json:"Image,omitempty"`
	User   string            `json:"User,omitempty"`
	Env    []string          `json:"Env,omitempty"`
	Labels map[string]string `json:"Labels,omitempty"`
}

type ContainerDetailsState struct {
	Status    string `json:"Status,omitempty"`
	StartedAt string `json:"StartedAt,omitempty"`
}

func (l *LoftStarter) startDocker(ctx context.Context, name string) error {
	l.Log.Infof(product.Replace("Starting loft in Docker..."))

	// prepare installation
	err := l.prepareDocker()
	if err != nil {
		return err
	}

	// try to find loft container
	containerID, err := l.findLoftContainer(ctx, name)
	if err != nil {
		return err
	}

	// check if container is there
	if containerID != "" && l.Reset || l.Upgrade {
		l.Log.Info(product.Replace("Existing Loft instance found."))
		err = l.uninstallDocker(ctx, containerID)
		if err != nil {
			return err
		}

		containerID = ""
	}

	// Use default password if none is set
	if l.Password == "" {
		l.Password = getMachineUID(l.Log)
	}

	// check if is installed
	if containerID != "" {
		l.Log.Info(product.Replace("Existing Loft instance found. Run with --upgrade to apply new configuration"))
		return l.successDocker(ctx, containerID)
	}

	// Install Loft
	l.Log.Info(product.Replace("Welcome to Loft!"))
	l.Log.Info(product.Replace("This installer will help you configure and deploy Loft."))

	// Get email
	email, err := l.getEmail()
	if err != nil {
		return err
	}

	// make sure we are ready for installing
	containerID, err = l.runLoftInDocker(ctx, name, email)
	if err != nil {
		return err
	} else if containerID == "" {
		return ErrMissingContainer
	}

	return l.successDocker(ctx, containerID)
}

func (l *LoftStarter) successDocker(ctx context.Context, containerID string) error {
	if l.NoWait {
		return nil
	}

	// wait until Loft is ready
	host, err := l.waitForLoftDocker(ctx, containerID)
	if err != nil {
		return err
	}

	// wait for domain to become reachable
	l.Log.Infof(product.Replace("Wait for Loft to become available at %s..."), host)
	waitErr := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute*10, true, func(ctx context.Context) (bool, error) {
		return clihelper.IsLoftReachable(ctx, host)
	})
	if waitErr != nil {
		return fmt.Errorf(product.Replace("error waiting for loft: %w"), err)
	}

	// print success message
	PrintSuccessMessageDockerInstall(host, l.Password, l.Log)
	return nil
}

func PrintSuccessMessageDockerInstall(host, password string, log log.Logger) {
	url := "https://" + host
	log.WriteString(logrus.InfoLevel, fmt.Sprintf(product.Replace(`


##########################   LOGIN   ############################

Username: `+ansi.Color("admin", "green+b")+`
Password: `+ansi.Color(password, "green+b")+`

Login via UI:  %s
Login via CLI: %s

#################################################################

Loft was successfully installed and can now be reached at: %s

Thanks for using Loft!
`),
		ansi.Color(url, "green+b"),
		ansi.Color(product.Replace(`loft login `)+url, "green+b"),
		url,
	))
}

func (l *LoftStarter) waitForLoftDocker(ctx context.Context, containerID string) (string, error) {
	l.Log.Info(product.Replace("Wait for Loft to become available..."))

	// check for local port
	containerDetails, err := l.inspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	} else if len(containerDetails.NetworkSettings.Ports) > 0 && len(containerDetails.NetworkSettings.Ports["10443/tcp"]) > 0 {
		return "localhost:" + containerDetails.NetworkSettings.Ports["10443/tcp"][0].HostPort, nil
	}

	// check if no tunnel
	if l.NoTunnel {
		return "", ErrLoftNotReachable
	}

	// wait for router
	url := ""
	waitErr := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute*10, true, func(ctx context.Context) (bool, error) {
		url, err = l.findLoftRouter(ctx, containerID)
		if err != nil {
			return false, nil
		}

		return true, nil
	})
	if waitErr != nil {
		return "", fmt.Errorf("error waiting for loft router domain: %w", err)
	}

	return url, nil
}

func (l *LoftStarter) findLoftRouter(ctx context.Context, id string) (string, error) {
	out, err := l.buildDockerCmd(ctx, "exec", id, "cat", "/var/lib/loft/loft-domain.txt").Output()
	if err != nil {
		return "", WrapCommandError(out, err)
	}

	return strings.TrimSpace(string(out)), nil
}

func (l *LoftStarter) prepareDocker() error {
	// test for helm and kubectl
	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("seems like docker is not installed. Docker is required for the installation of loft. Please visit https://docs.docker.com/engine/install/ for install instructions")
	}

	output, err := exec.Command("docker", "ps").CombinedOutput()
	if err != nil {
		return fmt.Errorf("seems like there are issues with your docker cli: \n\n%s", output)
	}

	return nil
}

func (l *LoftStarter) uninstallDocker(ctx context.Context, id string) error {
	l.Log.Infof(product.Replace("Uninstalling loft..."))

	// stop container
	out, err := l.buildDockerCmd(ctx, "stop", id).Output()
	if err != nil {
		return fmt.Errorf("stop container: %w", WrapCommandError(out, err))
	}

	// remove container
	out, err = l.buildDockerCmd(ctx, "rm", id).Output()
	if err != nil {
		return fmt.Errorf("remove container: %w", WrapCommandError(out, err))
	}

	return nil
}

func (l *LoftStarter) runLoftInDocker(ctx context.Context, name, email string) (string, error) {
	args := []string{"run", "-d", "--name", name}
	if l.NoTunnel {
		args = append(args, "--env", "DISABLE_LOFT_ROUTER=true")
	}
	if l.Password != "" {
		args = append(args, "--env", "ADMIN_PASSWORD_HASH="+hash.String(l.Password))
	}
	if email != "" {
		args = append(args, "--env", "ADMIN_EMAIL="+email)
	}
	if l.Product != "" {
		args = append(args, "--env", "PRODUCT="+l.Product)
		if l.Product == "devpod-pro" {
			_, err := os.Stat("/var/run/docker.sock")
			if err == nil {
				args = append(args, "-v", "/var/run/docker.sock:/var/run/docker.sock")

				// run as root otherwise we get permission errors
				args = append(args, "-u", "root")
			}
		}
	}

	// mount the loft lib
	args = append(args, "-v", "/var/lib/loft:/var/lib/loft")

	// set port
	if l.LocalPort != "" {
		args = append(args, "-p", l.LocalPort+":10443")
	}

	// set extra args
	args = append(args, l.DockerArgs...)

	// set image
	if l.Version != "" {
		args = append(args, "ghcr.io/loft-sh/loft:"+strings.TrimPrefix(l.Version, "v"))
	} else {
		args = append(args, "ghcr.io/loft-sh/loft:latest")
	}

	l.Log.Infof("Start Loft via 'docker %s'", strings.Join(args, " "))
	runCmd := l.buildDockerCmd(ctx, args...)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	err := runCmd.Run()
	if err != nil {
		return "", err
	}

	return l.findLoftContainer(ctx, name)
}

func (l *LoftStarter) inspectContainer(ctx context.Context, id string) (*ContainerDetails, error) {
	args := []string{"inspect", "--type", "container"}
	args = append(args, id)
	out, err := l.buildDockerCmd(ctx, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", WrapCommandError(out, err))
	}

	containerDetails := []*ContainerDetails{}
	err = json.Unmarshal(out, &containerDetails)
	if err != nil {
		return nil, fmt.Errorf("parse inspect output: %w", err)
	} else if len(containerDetails) == 0 {
		return nil, fmt.Errorf("coudln't find container %s", id)
	}

	return containerDetails[0], nil
}

func (l *LoftStarter) findLoftContainer(ctx context.Context, name string) (string, error) {
	args := []string{"ps", "-q", "-a", "-f", "name=^" + name + "$"}
	out, err := l.buildDockerCmd(ctx, args...).Output()
	if err != nil {
		// fallback to manual search
		return "", fmt.Errorf("error finding container: %w", WrapCommandError(out, err))
	}

	arr := []string{}
	scan := scanner.NewScanner(bytes.NewReader(out))
	for scan.Scan() {
		arr = append(arr, strings.TrimSpace(scan.Text()))
	}
	if len(arr) == 0 {
		return "", nil
	}

	return arr[0], nil
}

func (l *LoftStarter) buildDockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd
}

func WrapCommandError(stdout []byte, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		stdout: stdout,
		err:    err,
	}
}

type Error struct {
	stdout []byte
	err    error
}

func (e *Error) Error() string {
	message := ""
	if len(e.stdout) > 0 {
		message += string(e.stdout) + "\n"
	}

	var exitError *exec.ExitError
	if errors.As(e.err, &exitError) && len(exitError.Stderr) > 0 {
		message += string(exitError.Stderr) + "\n"
	}

	return message + e.err.Error()
}

func getMachineUID(log log.Logger) string {
	id, err := machineid.ID()
	if err != nil {
		id = "error"
		if log != nil {
			log.Debugf("Error retrieving machine uid: %v", err)
		}
	}
	// get $HOME to distinguish two users on the same machine
	// will be hashed later together with the ID
	home, err := homedir.Dir()
	if err != nil {
		home = "error"
		if log != nil {
			log.Debugf("Error retrieving machine home: %v", err)
		}
	}
	mac := hmac.New(sha256.New, []byte(id))
	mac.Write([]byte(home))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
