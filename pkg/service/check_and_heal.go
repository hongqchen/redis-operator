package service

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/util"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CheckAndHealer interface {
	CheckNumberOfMasters(cRedis *v1beta1.CustomRedis) error
}

type CheckAndHeal struct {
	logger       logr.Logger
	k8sService   kubernetesServicer
	redisService RedisServicer
}

func NewCheckAndHeal(cl client.Client, logger logr.Logger) *CheckAndHeal {
	return &CheckAndHeal{
		logger:       logger,
		k8sService:   NewkubernetesService(cl, logger),
		redisService: NewRedisService(),
	}
}

// CheckNumberOfMasters 检查集群中 master 节点数量
func (ch *CheckAndHeal) CheckNumberOfMasters(cRedis *v1beta1.CustomRedis) error {
	ch.logger.Info("Checking the number of cluster masters")
	masterIPs, err := ch.k8sService.GetMasterIPs(cRedis)
	if err != nil {
		return err
	}

	switch len(masterIPs) {
	case 0:
		return ch.healNoMasters(cRedis)
	case 1:
		return nil
	default:
		return ch.healManyMasters(cRedis)
	}
}

func (ch *CheckAndHeal) healNoMasters(cRedis *v1beta1.CustomRedis) error {
	ch.logger.Info("Healing no master")
	redisNodes, err := ch.k8sService.GetStatefulsetReadyPods(cRedis.Name, cRedis.Namespace)
	if err != nil {
		return err
	}

	// 如果是首次创建，则设置创建时间最长的 Pod 为 master
	if cRedis.Status.Phase == util.CustomRedisCreating {
		return ch.redisService.SetOldestAsMaster(cRedis, redisNodes)
	}

	// master-slave
	// 如果运行状态， master 被删，导致集群中只剩 slave 节点，抛出异常，提醒人员手动修复
	if cRedis.Spec.ClusterMode == v1beta1.MasterSlave {
		return util.NoMasterErr
	}

	// sentinel
	// 如果运行状态， master 被删，导致集群中只剩 slave 节点
	// 此时哨兵正在选举新的 master，抛出异常，等待 requeue
	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		return util.MasterBeElectingErr
	}

	return errors.New("unknown error")
}

func (ch *CheckAndHeal) healManyMasters(cRedis *v1beta1.CustomRedis) error {
	name := cRedis.Name
	namespace := cRedis.Namespace

	redisNodes, err := ch.k8sService.GetStatefulsetReadyPods(name, namespace)
	if err != nil {
		return err
	}

	// 如果是首次创建，则设置创建时间最长的 Pod 为 master
	if cRedis.Status.Phase == util.CustomRedisCreating {
		return ch.redisService.SetOldestAsMaster(cRedis, redisNodes)
	}

	// 非首次创建
	// master-slave，抛出异常，提醒需要人为选举一个 master，其他设置为 slave（手动操作）
	if cRedis.Spec.ClusterMode == v1beta1.MasterSlave {
		return util.ManyMastersErr
	}

	// sentinel，找出 sentinel 选出的 master，对比其IP，将其他的 master 设置为 slave（自动操作）
	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		// 获取 sentinel 节点 IP
		monitorIP, err := ch.getSentinelMonitor(cRedis)
		if err != nil {
			return err
		}

		// 存在的 masterIP 依次和 sentinel monitor 对比，不一致则设置为 slave
		// 获取 master pod IP
		masterIPs, err := ch.k8sService.GetMasterIPs(cRedis)
		if err != nil {
			return err
		}

		for _, masterIP := range masterIPs {
			if masterIP == monitorIP {
				// 当前 pod 角色为 master
				// sentinel 监听的 IP 和当前 IP 一致
				// 跳过
				continue
			}

			// IP 不一致，redis node(Pod)设置为 slave
			if err := ch.redisService.SetAsSlave(cRedis, masterIP, monitorIP); err != nil {
				return err
			}
		}
		return nil
	}

	return util.UnknownErr
}

func (ch *CheckAndHeal) getSentinelMonitor(cRedis *v1beta1.CustomRedis) (string, error) {
	name := cRedis.Name
	namespace := cRedis.Namespace
	sentinelName := fmt.Sprintf("%s-%s", name, util.SentinelResourceSuffix)

	// 获取 sentinel pod list
	sentinelPods, err := ch.k8sService.GetDeploymentReadyPods(sentinelName, namespace)
	if err != nil {
		return "", err
	}

	// 废弃
	// 获取 sentinel 集群监控的 master IP
	//monitorIPs := make(map[string]struct{})
	//for _, pod := range sentinelPods {
	//	storedMonitor, _, err := ch.redisService.GetSentinelMonitor(cRedis, pod.Status.PodIP)
	//	if err != nil {
	//		return "", err
	//	}
	//
	//	_, exists := monitorIPs[storedMonitor]
	//	if exists {
	//		continue
	//	}
	//	monitorIPs[storedMonitor] = struct{}{}
	//}
	//
	//if len(monitorIPs) == 1 {
	//	for key := range monitorIPs {
	//		return key, nil
	//	}
	//}
	//
	//return "", util.ManyMonitorsOnSentinelErr

	monitorIP := ""
	for _, pod := range sentinelPods {
		storedMonitor, _, err := ch.redisService.GetSentinelMonitor(cRedis, pod.Status.PodIP)
		if err != nil {
			return "", err
		}

		if storedMonitor == "127.0.0.1" {
			continue
		}

		monitorIP = storedMonitor
		break
	}

	return monitorIP, nil
}
