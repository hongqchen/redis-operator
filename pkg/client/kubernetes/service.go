package kubernetes

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Servicer = (*Service)(nil)

type Servicer interface {
	GetService(name, namespace string) (*corev1.Service, error)
	CreateService(service *corev1.Service) error
	UpdateService(service *corev1.Service) error
}

type Service struct {
	cl client.Client
}

func NewService(cl client.Client) *Service {
	return &Service{cl: cl}
}

func (s *Service) GetService(name, namespace string) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := s.cl.Get(context.TODO(), client.ObjectKeyFromObject(service), service)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) CreateService(service *corev1.Service) error {
	return s.cl.Create(context.TODO(), service)
}

func (s *Service) UpdateService(service *corev1.Service) error {
	return s.cl.Update(context.TODO(), service)
}
