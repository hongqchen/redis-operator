package kubernetes

import "sigs.k8s.io/controller-runtime/pkg/client"

var _ Clienter = (*Client)(nil)

type Clienter interface {
	Configmaper
	Servicer
	Statefulseter
	Poder
	Deploymenter
}

type Client struct {
	Configmaper
	Servicer
	Statefulseter
	Poder
	Deploymenter
}

func NewClient(cl client.Client) *Client {
	return &Client{
		Configmaper:   NewConfigmap(cl),
		Servicer:      NewService(cl),
		Statefulseter: NewStatefulset(cl),
		Poder:         NewPod(cl),
		Deploymenter:  NewDeployment(cl),
	}
}
