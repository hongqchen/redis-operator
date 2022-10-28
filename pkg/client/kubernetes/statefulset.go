package kubernetes

import (
	"context"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Statefulseter = (*Statefulset)(nil)

type Statefulseter interface {
	GetStatefulset(name, namespace string) (*appv1.StatefulSet, error)
	CreateStatefulset(sts *appv1.StatefulSet) error
	UpdateStatefulset(sts *appv1.StatefulSet) error
}

type Statefulset struct {
	cl client.Client
}

func NewStatefulset(cl client.Client) *Statefulset {
	return &Statefulset{
		cl: cl,
	}
}

func (s *Statefulset) GetStatefulset(name, namespace string) (*appv1.StatefulSet, error) {
	statefulset := &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := s.cl.Get(context.TODO(), client.ObjectKeyFromObject(statefulset), statefulset)
	if err != nil {
		return nil, err
	}
	return statefulset, nil
}

func (s *Statefulset) CreateStatefulset(sts *appv1.StatefulSet) error {
	return s.cl.Create(context.TODO(), sts)
}

func (s *Statefulset) UpdateStatefulset(sts *appv1.StatefulSet) error {
	return s.cl.Update(context.TODO(), sts)
}
