package util

import "github.com/pkg/errors"

type CustomRedisPhase string

const (
	RedisConfigFileName  = "redis.conf"
	RedisConfigMountPath = "/redis/cm"

	SentinelConfigFileName = "sentinel.conf"
	SentinelResourceSuffix = "sentinel"
	SentinelPort           = 26379

	CustomRedisFailed   CustomRedisPhase = "failed"
	CustomRedisCreating CustomRedisPhase = "creating"
	CustomRedisScaling  CustomRedisPhase = "scaling"
	CustomRedisRunning  CustomRedisPhase = "running"
)

var (
	AllPodReadyErr      = errors.New("not all pods are ready")
	NoMasterErr         = errors.New("cluster has no master")
	MasterBeElectingErr = errors.New("master is being elected")
	ManyMastersErr      = errors.New("multiple masters exist")
	UnknownErr          = errors.New("unknown error")
	DeprecatedErr       = errors.New("deprecated master")
	//ManyMonitorsOnSentinelErr = errors.New("sentinel cluster listens on several different masters")
)
