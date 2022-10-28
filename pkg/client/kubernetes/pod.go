package kubernetes

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Poder = (*Pod)(nil)

type Poder interface {
	GetPod(name, namespace string) (*corev1.Pod, error)
	GetPods(namespace string, selector client.MatchingLabels) (corev1.PodList, error)
	UpdatePod(podObj *corev1.Pod) error
}

type Pod struct {
	cl client.Client
}

func NewPod(cl client.Client) *Pod {
	return &Pod{cl: cl}
}

func (p *Pod) GetPods(namespace string, selector client.MatchingLabels) (corev1.PodList, error) {
	pods := corev1.PodList{}
	if err := p.cl.List(context.TODO(), &pods, client.InNamespace(namespace), selector); err != nil {
		return corev1.PodList{}, err
	}

	return pods, nil
}

func (p *Pod) GetPod(name, namespace string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := p.cl.Get(context.TODO(), client.ObjectKeyFromObject(pod), pod)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (p *Pod) UpdatePod(podObj *corev1.Pod) error {
	return p.cl.Update(context.TODO(), podObj)
}
