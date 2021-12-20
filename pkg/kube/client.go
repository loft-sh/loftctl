package kube

import (
	kioskclient "github.com/loft-sh/agentapi/pkg/client/kiosk/clientset_generated/clientset"
	agentloftclient "github.com/loft-sh/agentapi/pkg/client/loft/clientset_generated/clientset"
	loftclient "github.com/loft-sh/api/pkg/client/clientset_generated/clientset"

	"github.com/pkg/errors"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Interface interface {
	kubernetes.Interface
	Loft() loftclient.Interface
	Kiosk() kioskclient.Interface
	Agent() agentloftclient.Interface
}

func NewForConfig(c *rest.Config) (Interface, error) {
	kubeClient, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "create kube client")
	}

	loftClient, err := loftclient.NewForConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "create loft client")
	}

	kioskClient, err := kioskclient.NewForConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "create kiosk client")
	}

	agentLoftClient, err := agentloftclient.NewForConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "create kiosk client")
	}

	return &client{
		Interface:       kubeClient,
		loftClient:      loftClient,
		kioskClient:     kioskClient,
		agentLoftClient: agentLoftClient,
	}, nil
}

type client struct {
	kubernetes.Interface
	loftClient      loftclient.Interface
	kioskClient     kioskclient.Interface
	agentLoftClient agentloftclient.Interface
}

func (c *client) Loft() loftclient.Interface {
	return c.loftClient
}

func (c *client) Kiosk() kioskclient.Interface {
	return c.kioskClient
}

func (c *client) Agent() agentloftclient.Interface {
	return c.agentLoftClient
}
