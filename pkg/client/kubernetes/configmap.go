package kubernetes

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Configmaper = (*Configmap)(nil)

type Configmaper interface {
	GetConfigmap(name, namespace string) (*corev1.ConfigMap, error)
	CreateConfigmap(configmap *corev1.ConfigMap) error
	UpdateConfigmap(configmap *corev1.ConfigMap) error
}

type Configmap struct {
	cl client.Client
}

func NewConfigmap(cl client.Client) *Configmap {
	return &Configmap{cl: cl}
}

func (cm *Configmap) GetConfigmap(name, namespace string) (*corev1.ConfigMap, error) {
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := cm.cl.Get(context.TODO(), client.ObjectKeyFromObject(configmap), configmap)
	if err != nil {
		return nil, err
	}
	return configmap, nil
}

func (cm *Configmap) CreateConfigmap(configmap *corev1.ConfigMap) error {
	return cm.cl.Create(context.TODO(), configmap)
}

func (cm *Configmap) UpdateConfigmap(configmap *corev1.ConfigMap) error {
	return cm.cl.Update(context.TODO(), configmap)
}
