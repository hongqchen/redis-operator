package service

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/client/kubernetes"
	"github.com/hongqchen/redis-operator/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ kubernetesServicer = (*KubernetesService)(nil)

type kubernetesServicer interface {
	// GetConfigmap configmap
	GetConfigmap(name, namespace string) (*corev1.ConfigMap, error)
	CreateConfigmap(configmap *corev1.ConfigMap) error
	UpdateConfigmap(configmap *corev1.ConfigMap) error

	// statefulset
	GetStatefulset(name, namespace string) (*appv1.StatefulSet, error)
	CreateStatefulset(sts *appv1.StatefulSet) error
	UpdateStatefulset(sts *appv1.StatefulSet) error

	// service
	GetService(name, namespace string) (*corev1.Service, error)
	CreateService(service *corev1.Service) error
	UpdateService(service *corev1.Service) error

	// deployment
	GetDeployment(name, namespace string) (*appv1.Deployment, error)
	CreateDeployment(deploy *appv1.Deployment) error
	UpdateDeployment(deploy *appv1.Deployment) error

	// GetReplicas 获取副本数
	GetReplicas(cRedis *v1beta1.CustomRedis) (int32, error)
	// GetStatefulsetReadyPods 获取 statefulset ready 的 pod 列表
	GetStatefulsetReadyPods(name, namespace string) ([]corev1.Pod, error)
	// GetDeploymentReadyPods 获取 deployment ready 的 Pod 列表
	GetDeploymentReadyPods(name, namespace string) ([]corev1.Pod, error)
	// GetIPsByMasterRole 获取 master 节点的 IP 列表
	GetMasterIPs(cRedis *v1beta1.CustomRedis) ([]string, error)

	UpdatePodIfExists(podObj *corev1.Pod) error
}

type KubernetesService struct {
	logger       logr.Logger
	k8sClient    kubernetes.Clienter
	redisService RedisServicer
}

func NewkubernetesService(cl client.Client, logger logr.Logger) *KubernetesService {
	return &KubernetesService{
		logger:       logger,
		k8sClient:    kubernetes.NewClient(cl),
		redisService: NewRedisService(logger),
	}
}

func (ks *KubernetesService) getObj(cRedis *v1beta1.CustomRedis) (interface{}, error) {
	if cRedis.Spec.ClusterMode == v1beta1.MasterSlave {
		return ks.k8sClient.GetStatefulset(cRedis.Name, cRedis.Namespace)
	}

	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		return ks.k8sClient.GetDeployment(fmt.Sprintf("%s-%s", cRedis.Name, util.SentinelResourceSuffix), cRedis.Namespace)
	}

	return "", fmt.Errorf("invalid cluster mode")
}

func (ks *KubernetesService) GetReplicas(cRedis *v1beta1.CustomRedis) (int32, error) {
	ks.logger.V(1).Info("Getting resource sepc.relicas")
	res, err := ks.getObj(cRedis)
	if err != nil {
		return 0, err
	}

	switch obj := res.(type) {
	case *appv1.StatefulSet:
		return *obj.Spec.Replicas, nil
	case *appv1.Deployment:
		return *obj.Spec.Replicas, nil
	default:
		return 0, fmt.Errorf("invalid resource type")
	}
}

// For example [{"name":"pod_name","ip":"pod_ip"},{"name2":"pod_name2","ip2":"pod_ip2"}]
func (ks *KubernetesService) GetStatefulsetReadyPods(name, namespace string) ([]corev1.Pod, error) {
	ks.logger.V(1).Info("Getting all ready pods with statefulset")
	var readyPods []corev1.Pod

	// 从 statefulset 获取 Pod selector
	storedSts, err := ks.k8sClient.GetStatefulset(name, namespace)
	if err != nil {
		return nil, err
	}
	selector := storedSts.Spec.Selector.MatchLabels

	// 通过 selector 获取 Pod 列表
	podsObj, err := ks.k8sClient.GetPods(namespace, selector)
	if err != nil {
		return nil, err
	}

	for _, pod := range podsObj.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp == nil {
			if len(pod.Status.Conditions) > 1 && pod.Status.Conditions[1].Status == "True" {
				readyPods = append(readyPods, pod)
			}
		}
	}

	return readyPods, nil
}

func (ks *KubernetesService) GetDeploymentReadyPods(name, namespace string) ([]corev1.Pod, error) {
	ks.logger.V(1).Info("Getting all ready pods with deployment")
	var readyPods []corev1.Pod

	storedObj, err := ks.k8sClient.GetDeployment(name, namespace)
	if err != nil {
		return nil, err
	}
	selector := storedObj.Spec.Selector.MatchLabels

	// 通过 selector 获取 Pod 列表
	podsObj, err := ks.k8sClient.GetPods(namespace, selector)
	if err != nil {
		return nil, err
	}

	for _, pod := range podsObj.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp == nil {
			if len(pod.Status.Conditions) > 1 && pod.Status.Conditions[1].Status == "True" {
				readyPods = append(readyPods, pod)
			}
		}
	}

	return readyPods, nil
}

func (ks *KubernetesService) GetMasterIPs(cRedis *v1beta1.CustomRedis) ([]string, error) {
	ks.logger.V(1).Info("Getting master IPs in cluster")
	var masterIPs []string

	pods, err := ks.GetStatefulsetReadyPods(cRedis.Name, cRedis.Namespace)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods {
		ip := pod.Status.PodIP
		ismaster, err := ks.redisService.IsMaster(cRedis, ip)
		if err != nil {
			return nil, err
		}

		if ismaster {
			masterIPs = append(masterIPs, ip)
		}
	}

	return masterIPs, nil
}

func (ks *KubernetesService) UpdatePodIfExists(podObj *corev1.Pod) error {
	ks.logger.V(1).Info("Updating pod")
	_, err := ks.k8sClient.GetPod(podObj.Name, podObj.Namespace)
	if err != nil {
		return err
	}

	podObj.ObjectMeta.ResourceVersion = ""
	return ks.k8sClient.UpdatePod(podObj)
}

func (ks *KubernetesService) GetConfigmap(name, namespace string) (*corev1.ConfigMap, error) {
	ks.logger.V(1).Info("Getting configmap")
	return ks.k8sClient.GetConfigmap(name, namespace)
}

func (ks *KubernetesService) CreateConfigmap(configmap *corev1.ConfigMap) error {
	ks.logger.V(1).Info("Creating configmap")
	return ks.k8sClient.CreateConfigmap(configmap)
}

func (ks *KubernetesService) UpdateConfigmap(configmap *corev1.ConfigMap) error {
	ks.logger.V(1).Info("Updating configmap")
	return ks.k8sClient.UpdateConfigmap(configmap)
}

func (ks *KubernetesService) GetStatefulset(name, namespace string) (*appv1.StatefulSet, error) {
	ks.logger.V(1).Info("Getting statefulset")
	return ks.k8sClient.GetStatefulset(name, namespace)
}

func (ks *KubernetesService) CreateStatefulset(sts *appv1.StatefulSet) error {
	ks.logger.V(1).Info("Creating statefulset")
	return ks.k8sClient.CreateStatefulset(sts)
}

func (ks *KubernetesService) UpdateStatefulset(sts *appv1.StatefulSet) error {
	ks.logger.V(1).Info("Updating statefulset")
	return ks.k8sClient.UpdateStatefulset(sts)
}

func (ks *KubernetesService) GetService(name, namespace string) (*corev1.Service, error) {
	ks.logger.V(1).Info("Getting service")
	return ks.k8sClient.GetService(name, namespace)
}

func (ks *KubernetesService) CreateService(service *corev1.Service) error {
	ks.logger.V(1).Info("Creating service")
	return ks.k8sClient.CreateService(service)
}

func (ks *KubernetesService) UpdateService(service *corev1.Service) error {
	ks.logger.V(1).Info("Updating service")
	return ks.k8sClient.UpdateService(service)
}

func (ks *KubernetesService) GetDeployment(name, namespace string) (*appv1.Deployment, error) {
	ks.logger.V(1).Info("Getting deployment")
	return ks.k8sClient.GetDeployment(name, namespace)
}

func (ks *KubernetesService) CreateDeployment(deploy *appv1.Deployment) error {
	ks.logger.V(1).Info("Creating deployment")
	return ks.k8sClient.CreateDeployment(deploy)
}

func (ks *KubernetesService) UpdateDeployment(deploy *appv1.Deployment) error {
	ks.logger.V(1).Info("Updating deployment")
	return ks.k8sClient.UpdateDeployment(deploy)
}
