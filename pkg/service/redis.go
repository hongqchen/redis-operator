package service

import (
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/client/redis"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	replicationOfMasterHostRE = regexp.MustCompile("master_host:([0-9.]+)")
)

var _ RedisServicer = (*RedisService)(nil)

type RedisServicer interface {
	// 直接与 redis client 交互
	GetReplication(cRedis *v1beta1.CustomRedis, ip string) (string, error)
	GetSentinelMonitor(cRedis *v1beta1.CustomRedis, sentienlIP string) (string, string, error)
	SetAsMaster(cRedis *v1beta1.CustomRedis, ip string) error
	SetAsSlave(cRedis *v1beta1.CustomRedis, slaveIP, masterIP string) error
	SetSentinelMonitor(cRedis *v1beta1.CustomRedis, sentinelIP, masterIP string) error

	GetReplicationOfMasterHost(cRedis *v1beta1.CustomRedis, ip string) (string, error)
	IsMaster(cRedis *v1beta1.CustomRedis, ip string) (bool, error)

	SetOldestAsMaster(cRedis *v1beta1.CustomRedis, pods []corev1.Pod) error
	//SetExceptOldestAsSlave(cRedis *v1beta1.CustomRedis, pods []corev1.Pod) error
}

type RedisService struct {
	logger logr.Logger
	client redis.Clienter
}

func NewRedisService(logger logr.Logger) *RedisService {
	return &RedisService{
		logger: logger,
		client: redis.NewClient(),
	}
}

func (rs *RedisService) GetReplication(cRedis *v1beta1.CustomRedis, ip string) (string, error) {
	rs.logger.V(1).Info("Getting replication info", "currentIP", ip)
	port, password, err := rs.getPortAndPassword(cRedis)
	if err != nil {
		return "", err
	}

	return rs.client.GetReplication(ip, port, password)
}

func (rs *RedisService) IsMaster(cRedis *v1beta1.CustomRedis, ip string) (bool, error) {
	rs.logger.V(1).Info("Trying to determine whether it is master", "currentIP", ip)
	replication, err := rs.GetReplication(cRedis, ip)
	if err != nil {
		return false, err
	}

	return strings.Contains(replication, "role:master"), nil
}

func (rs *RedisService) SetAsMaster(cRedis *v1beta1.CustomRedis, ip string) error {
	rs.logger.V(1).Info("Setting as master", "currentIP", ip)
	port, password, err := rs.getPortAndPassword(cRedis)
	if err != nil {
		return err
	}

	return rs.client.SetAsMaster(ip, port, password)
}

func (rs *RedisService) SetAsSlave(cRedis *v1beta1.CustomRedis, slaveIP, masterIP string) error {
	rs.logger.V(1).Info("Setting as slave", "currentIP", slaveIP, "masterIP", masterIP)
	port, password, err := rs.getPortAndPassword(cRedis)
	if err != nil {
		return err
	}

	return rs.client.SetAsSlave(slaveIP, masterIP, port, password)
}

// Set the pod with the longest creation time as the master
func (rs *RedisService) SetOldestAsMaster(cRedis *v1beta1.CustomRedis, pods []corev1.Pod) error {
	rs.logger.V(1).Info("Setting the oldest pod as master")
	if len(pods) < 1 {
		return errors.New("no ready pods available")
	}

	// Sort by creation time in ascending order
	// slice[0] is oldest pod
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
	})

	masterIP := ""
	for _, pod := range pods {
		// Check that the pod is ready, otherwise ignore it
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			continue
		}

		if masterIP == "" {
			// set as master node
			masterIP = pod.Status.PodIP
			if err := rs.SetAsMaster(cRedis, masterIP); err != nil {
				return err
			}
		} else {
			// set as slave node
			if err := rs.SetAsSlave(cRedis, pod.Status.PodIP, masterIP); err != nil {
				return err
			}
		}
	}

	return nil
}

//func (rs *RedisService) SetExceptOldestAsSlave(cRedis *v1beta1.CustomRedis, pods []corev1.Pod) error {
//	rs.logger.V(1).Info("Setting the oldest pod as master")
//	if len(pods) < 1 {
//		return errors.New("no ready pods available")
//	}
//
//	// Sort by creation time in ascending order
//	// slice[0] is oldest pod
//	sort.Slice(pods, func(i, j int) bool {
//		return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
//	})
//
//	masterIP := pods[0].Status.PodIP
//	slavePods := pods[1:]
//	for _, pod := range slavePods {
//		if err := rs.SetAsSlave(cRedis, pod.Status.PodIP, masterIP); err != nil {
//			return err
//		}
//	}
//
//	return nil
//}

func (rs *RedisService) GetSentinelMonitor(cRedis *v1beta1.CustomRedis, sentienlIP string) (string, string, error) {
	rs.logger.V(1).Info("Getting sentinel monitor info", "currentIP", sentienlIP)
	_, password, _ := rs.getPortAndPassword(cRedis)
	return rs.client.GetSentinelMonitor(sentienlIP, password)
}

func (rs *RedisService) SetSentinelMonitor(cRedis *v1beta1.CustomRedis, sentinelIP, masterIP string) error {
	rs.logger.V(1).Info("Setting the monitor for sentinel nodes", "sentinelIP", sentinelIP, "masterIP", masterIP)
	port, password, err := rs.getPortAndPassword(cRedis)
	if err != nil {
		return err
	}
	quorum := strconv.Itoa(int(*cRedis.Spec.SentinelNum)/2 + 1)

	monitor := map[string]interface{}{
		"masterIP": masterIP,
		"port":     port,
		"quorum":   quorum,
	}
	return rs.client.SetSentinelMonitor(sentinelIP, password, monitor)
}

func (rs *RedisService) GetReplicationOfMasterHost(cRedis *v1beta1.CustomRedis, ip string) (string, error) {
	rs.logger.V(1).Info("Getting the masterIP of the sentinel node", "sentinelIP", ip)
	replication, err := rs.GetReplication(cRedis, ip)
	if err != nil {
		return "", err
	}

	matchRes := replicationOfMasterHostRE.FindStringSubmatch(replication)
	if len(matchRes) == 0 {
		return "", nil
	}

	return matchRes[1], nil
}

func (rs *RedisService) getPortAndPassword(cRedis *v1beta1.CustomRedis) (int32, string, error) {
	password := cRedis.Spec.RedisConfig["requirepass"]

	// if cr.Spec.ClusterMode == "sentinel" {
	//	return 0, password, nil
	// }

	portString, exists := cRedis.Spec.RedisConfig["port"]
	if !exists {
		return 0, "", errors.New("redis conf has no port")
	}

	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return 0, "", errors.New("value of port is invalid")
	}

	return int32(portInt), password, nil
}
