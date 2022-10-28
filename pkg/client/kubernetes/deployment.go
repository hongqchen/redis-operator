package kubernetes

import (
	"context"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Deploymenter = (*Deployment)(nil)

type Deploymenter interface {
	GetDeployment(name, namespace string) (*appv1.Deployment, error)
	CreateDeployment(deploy *appv1.Deployment) error
	UpdateDeployment(deploy *appv1.Deployment) error
}

type Deployment struct {
	cl client.Client
}

func NewDeployment(cl client.Client) *Deployment {
	return &Deployment{
		cl: cl,
	}
}

func (d *Deployment) GetDeployment(name, namespace string) (*appv1.Deployment, error) {
	deploy := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := d.cl.Get(context.TODO(), client.ObjectKeyFromObject(deploy), deploy)
	if err != nil {
		return nil, err
	}
	return deploy, nil
}

func (d *Deployment) CreateDeployment(deploy *appv1.Deployment) error {
	return d.cl.Create(context.TODO(), deploy)
}

func (d *Deployment) UpdateDeployment(deploy *appv1.Deployment) error {
	return d.cl.Update(context.TODO(), deploy)
}
