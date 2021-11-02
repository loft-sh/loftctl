package task

import (
	"context"
	"fmt"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	"github.com/loft-sh/apiserver/pkg/builders"
	"github.com/loft-sh/loftctl/pkg/kube"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/pkg/errors"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/loft-sh/api/pkg/apis/management/install" // Install the management group
)

func StreamTask(ctx context.Context, managementClient kube.Interface, task *managementv1.Task, out io.Writer, log log.Logger) (err error) {
	// cleanup on ctrl+c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		<-c
		_ = managementClient.Loft().ManagementV1().Tasks().Delete(context.TODO(), task.Name, metav1.DeleteOptions{})
		os.Exit(1)
	}()

	defer func() {
		signal.Stop(c)
	}()

	log.Infof("Waiting for task to start...")
	createdTask, err := managementClient.Loft().ManagementV1().Tasks().Create(ctx, task, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create task")
	}

	err = wait.PollImmediate(time.Second, time.Minute*5, func() (done bool, err error) {
		task, err := managementClient.Loft().ManagementV1().Tasks().Get(ctx, createdTask.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return task.Status.PodPhase == corev1.PodRunning || task.Status.PodPhase == corev1.PodSucceeded || task.Status.PodPhase == corev1.PodFailed, nil
	})
	if err != nil {
		return errors.Wrap(err, "wait for task")
	}

	// now stream the logs
	request := managementClient.Loft().ManagementV1().RESTClient().Get().Name(createdTask.Name).Resource("tasks").SubResource("log").VersionedParams(&managementv1.TaskLogOptions{
		Follow: true,
	}, runtime.NewParameterCodec(builders.ParameterScheme))
	if request.URL().String() == "" {
		return errors.New("Request url is empty")
	}

	reader, err := request.Stream(ctx)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	// check task result
	task, err = managementClient.Loft().ManagementV1().Tasks().Get(ctx, createdTask.Name, metav1.GetOptions{})
	if err != nil {
		return err
	} else if task.Status.PodPhase == corev1.PodFailed {
		return fmt.Errorf("task failed")
	}

	return nil
}
