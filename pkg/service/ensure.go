package service

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/util"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Ensurer interface {
	// 保证资源被创建，并且符合 CustomRedis.spec
	EnsureConfigmap(cRedis *v1beta1.CustomRedis) error
	EnsureStatefulset(cRedis *v1beta1.CustomRedis) error
	EnsureService(cRedis *v1beta1.CustomRedis) error
	EnsureDeployment(cRedis *v1beta1.CustomRedis) error

	EnsurePodOwner(cRedis *v1beta1.CustomRedis) error

	// 确认资源状态正常，所有 pod ready
	EnsurePodReady(cRedis *v1beta1.CustomRedis) error
	// 确认 sentinel 监听了正确 master IP
	EnsureSentinelMonitor(cRedis *v1beta1.CustomRedis) error
	// 确认 slave 节点监听的 master 是正确的 IP
	EnsureSlaveOfMaster(cRedis *v1beta1.CustomRedis) error
	// 为不同角色的 Pod 添加 label
	EnsureLabels(cRedis *v1beta1.CustomRedis) error
}

type Ensure struct {
	logger       logr.Logger
	generate     generater
	k8sService   kubernetesServicer
	redisService RedisServicer
}

func NewEnsure(cl client.Client, logger logr.Logger) *Ensure {
	return &Ensure{
		logger:       logger,
		generate:     newGenerate(),
		k8sService:   NewkubernetesService(cl, logger),
		redisService: NewRedisService(),
	}
}

func (e *Ensure) EnsureConfigmap(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring configmap")
	baseCm := &corev1.ConfigMap{}

	if cRedis.Spec.ClusterMode == v1beta1.MasterSlave {
		baseCm = e.generate.configmap(cRedis)
	}

	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		baseCm = e.generate.configmapForSentinel(cRedis)
	}

	if _, err := e.k8sService.GetConfigmap(cRedis.Name, cRedis.Namespace); err != nil {
		if apierror.IsNotFound(err) {
			// configmap 不存在，需要创建
			return e.k8sService.CreateConfigmap(baseCm)
		}
		return err
	}

	// configmap 已存在，更新
	return e.k8sService.UpdateConfigmap(baseCm)
}

func (e *Ensure) EnsureStatefulset(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring statefulset")
	sts := e.generate.statefulset(cRedis)

	if _, err := e.k8sService.GetStatefulset(cRedis.Name, cRedis.Namespace); err != nil {
		if apierror.IsNotFound(err) {
			return e.k8sService.CreateStatefulset(sts)
		}
		return err
	}

	return e.k8sService.UpdateStatefulset(sts)
}

func (e *Ensure) EnsureService(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring service")
	namespace := cRedis.Namespace

	services := e.generate.service(cRedis)
	for serviceName, serviceObj := range services {
		svcName := serviceName
		svc := serviceObj
		if _, err := e.k8sService.GetService(svcName, namespace); err != nil {
			if !apierror.IsNotFound(err) {
				return err
			}

			if err := e.k8sService.CreateService(svc); err != nil {
				return err
			}
		} else {
			return e.k8sService.UpdateService(svc)
		}
	}

	return nil
}

func (e *Ensure) EnsureDeployment(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring deployment(sentinel cluster)")
	deploy := e.generate.deployment(cRedis)

	if _, err := e.k8sService.GetDeployment(fmt.Sprintf("%s-%s", cRedis.Name, util.SentinelResourceSuffix), cRedis.Namespace); err != nil {
		if apierror.IsNotFound(err) {
			return e.k8sService.CreateDeployment(deploy)
		}
		return err
	}
	return e.k8sService.UpdateDeployment(deploy)
}

func (e *Ensure) EnsurePodReady(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring all pods are ready")
	switch cRedis.Spec.ClusterMode {
	case v1beta1.MasterSlave:
		pods, err := e.k8sService.GetStatefulsetReadyPods(cRedis.Name, cRedis.Namespace)
		if err != nil {
			return err
		}

		if len(pods) != int(*cRedis.Spec.Replicas) {
			return util.AllPodReadyErr
		}
		return nil
	case v1beta1.Sentinel:
		// 判断 ready redis Pod 数量是否匹配 spec.replices
		redisNodes, err := e.k8sService.GetStatefulsetReadyPods(cRedis.Name, cRedis.Namespace)
		if err != nil {
			return err
		}
		if len(redisNodes) != int(*cRedis.Spec.Replicas) {
			return util.AllPodReadyErr
		}

		// 判断哨兵节点个数是否满足 spec.sentinelnums 的一半以上
		sentinelNodes, err := e.k8sService.GetDeploymentReadyPods(fmt.Sprintf("%s-%s", cRedis.Name, util.SentinelResourceSuffix), cRedis.Namespace)
		if len(sentinelNodes) != int(*cRedis.Spec.SentinelNum) {
			return util.AllPodReadyErr
		}
		return nil
	}

	return errors.New("unknown cluster mode")

}

func (e *Ensure) EnsurePodOwner(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring a second owner references for pod")
	name := cRedis.Name
	namespace := cRedis.Namespace
	sentinelName := fmt.Sprintf("%s-%s", cRedis.Name, util.SentinelResourceSuffix)

	allPods := []corev1.Pod{}

	// 获取相关联的 redis Pods
	pods, err := e.k8sService.GetStatefulsetReadyPods(name, namespace)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		allPods = append(allPods, pod)
	}

	// 如果是哨兵模式，哨兵节点 Pod 也添加第二 owner
	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		sentinelPods, err := e.k8sService.GetDeploymentReadyPods(sentinelName, namespace)
		if err != nil {
			return err
		}
		for _, sentinelPod := range sentinelPods {
			allPods = append(allPods, sentinelPod)
		}
	}

	// 遍历所有 Pod，添加 cr 所属 owner
	for _, pod := range allPods {
		podObjDeepCopy := pod.DeepCopy()
		storedOwn := podObjDeepCopy.ObjectMeta.GetOwnerReferences()

		// 判断当前 pod 是否已经存在 crd 相关的 OwnerReferences
		isNeedNewOwner := true
		for _, own := range storedOwn {
			if own.Kind == "CustomRedis" && own.Name == name {
				isNeedNewOwner = false
				break
			}
		}

		// 已经存在，跳过
		if !isNeedNewOwner {
			continue
		}

		// 构建新的 OwnerReferences
		isController := false
		newOwner := e.generate.createOwnerReference(cRedis)[0]
		newOwner.Controller = &isController

		storedOwn = append(storedOwn, newOwner)

		// 替换原有的 OwnerReferences
		podObjDeepCopy.ObjectMeta.OwnerReferences = storedOwn
		// 更新 pod 对象
		if err := e.k8sService.UpdatePodIfExists(podObjDeepCopy); err != nil {
			return err
		}
	}

	return nil
}

func (e *Ensure) EnsureSentinelMonitor(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring that sentinel listens to the correct master IP")
	// 获取当前集群中的 master 节点列表
	// 如果节点个数 ！= 1
	// 返回错误，重新触发 reconcile
	// 为了重新调用 CheckNumberOfMasters 方法，确保最后只有一个 master
	masterIPs, err := e.k8sService.GetMasterIPs(cRedis)
	if err != nil {
		return err
	}
	if len(masterIPs) != 1 {
		return util.ManyMastersErr
	}

	masterIP := masterIPs[0]

	name := cRedis.Name
	namespace := cRedis.Namespace
	sentinelName := fmt.Sprintf("%s-%s", name, util.SentinelResourceSuffix)

	sentinelPods, err := e.k8sService.GetDeploymentReadyPods(sentinelName, namespace)
	if err != nil {
		return err
	}

	for _, sentinelPod := range sentinelPods {
		sentinelIP := sentinelPod.Status.PodIP

		monitorIP, _, err := e.redisService.GetSentinelMonitor(cRedis, sentinelIP)
		if err != nil {
			return err
		}
		if monitorIP == "127.0.0.1" || monitorIP != masterIP {
			// 设置 sentinel monitor 为实际的 master IP
			if err := e.redisService.SetSentinelMonitor(cRedis, sentinelIP, masterIP); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *Ensure) EnsureSlaveOfMaster(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring all slave pods are listening to the correct master")
	masterIPs, err := e.k8sService.GetMasterIPs(cRedis)
	if err != nil {
		return err
	}
	if len(masterIPs) != 1 {
		return util.ManyMastersErr
	}

	masterIP := masterIPs[0]
	name := cRedis.Name
	namespace := cRedis.Namespace

	redisNodes, err := e.k8sService.GetStatefulsetReadyPods(name, namespace)
	if err != nil {
		return err
	}

	for _, redisNode := range redisNodes {
		slaveIP := redisNode.Status.PodIP
		if slaveIP == masterIP {
			continue
		}

		storedMaster, err := e.redisService.GetReplicationOfMasterHost(cRedis, slaveIP)
		if err != nil {
			return err
		}

		if storedMaster == masterIP {
			continue
		}

		if err := e.redisService.SetAsSlave(cRedis, slaveIP, masterIP); err != nil {
			return err
		}
	}

	return nil
}

func (e *Ensure) EnsureLabels(cRedis *v1beta1.CustomRedis) error {
	e.logger.Info("Ensuring pod's label")
	name := cRedis.Name
	namespace := cRedis.Namespace
	//sentinelName := fmt.Sprintf("%s-%s", name, util.SentinelResourceSuffix)

	pods, err := e.k8sService.GetStatefulsetReadyPods(name, namespace)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		podObj := pod.DeepCopy()
		ip := podObj.Status.PodIP
		ismaster, err := e.redisService.IsMaster(cRedis, ip)
		if err != nil {
			return err
		}
		if ismaster {
			// 添加 role: master label
			podObj.ObjectMeta.Labels["redis.hongqchen/role"] = "master"
		} else {
			// 添加 role: slave label
			podObj.ObjectMeta.Labels["redis.hongqchen/role"] = "slave"
		}
		if err := e.k8sService.UpdatePodIfExists(podObj); err != nil {
			return err
		}
	}

	// sentinel 模式，直接为所有 Pod 添加 role: sentinel label
	//if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
	//	sentinelPods, err := e.k8sService.GetDeploymentReadyPods(sentinelName, namespace)
	//	if err != nil {
	//		return err
	//	}
	//
	//	for _, sentinelPod := range sentinelPods {
	//		sentinelPodObj := sentinelPod.DeepCopy()
	//		sentinelPodObj.ObjectMeta.Labels["redis.hongqchen/role"] = "sentinel"
	//
	//		if err := e.k8sService.UpdatePodIfExists(sentinelPodObj); err != nil {
	//			return err
	//		}
	//	}
	//}

	return nil
}
