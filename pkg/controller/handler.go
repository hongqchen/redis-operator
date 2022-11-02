package controller

import (
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/service"
	"github.com/hongqchen/redis-operator/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type RedisHandler struct {
	logger logr.Logger
	ensure service.Ensurer
	check  service.CheckAndHealer
}

func NewRedisHandler(cl client.Client, logger logr.Logger) *RedisHandler {
	return &RedisHandler{
		logger: logger,
		ensure: service.NewEnsure(cl, logger),
		check:  service.NewCheckAndHeal(cl, logger),
	}
}

func (rh *RedisHandler) Sync(cRedis *v1beta1.CustomRedis) time.Duration {
	// 判断不同模式集群
	switch cRedis.Spec.ClusterMode {
	case v1beta1.MasterSlave:
		rh.logger.V(1).Info("Starting master-slave resource sync action")
		return util.ErrorHandle(rh.logger, rh.syncMasterSlave(cRedis))
	case v1beta1.Sentinel:
		rh.logger.V(1).Info("Starting sentinel resource sync action")
		return util.ErrorHandle(rh.logger, rh.syncSentinel(cRedis))
	}

	return 0
}

func (rh *RedisHandler) syncMasterSlave(cRedis *v1beta1.CustomRedis) error {
	// 调用 ensure 方法，确保资源已经创建
	if err := rh.ensure.EnsureConfigmap(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureStatefulset(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsurePodReadyForStatefulset(cRedis); err != nil {
		return err
	}
	// 额外增加一个方法，为相关联的 Pod 添加 cr 作为第二个 owner
	// 目的是监听相关 Pod 事件，以触发 reconcile
	// 来更新相应的 Label、监听地址等
	if err := rh.ensure.EnsurePodOwner(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureService(cRedis); err != nil {
		return err
	}
	// 调用 check 方法，确保状态符合预期
	if err := rh.check.CheckNumberOfMasters(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureSlaveOfMaster(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureLabels(cRedis); err != nil {
		return err
	}

	return nil
}

func (rh *RedisHandler) syncSentinel(cRedis *v1beta1.CustomRedis) error {
	if err := rh.syncMasterSlave(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureDeployment(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsurePodReadyForDeployment(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureSentinelMonitor(cRedis); err != nil {
		return err
	}
	if err := rh.ensure.EnsureSlaveOfMaster(cRedis); err != nil {
		return err
	}

	return nil
}
