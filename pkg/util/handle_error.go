package util

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"time"
)

func ErrorHandle(logger logr.Logger, err error) time.Duration {
	if err == nil {
		return 0
	}

	// statefulset 未创建，入队，等待下一次 reconcile
	if apierror.IsNotFound(err) {
		fmt.Println(err.Error())
		return 1 * time.Second
	}

	// pod is being created
	if errors.Is(err, AllPodReadyErr) {
		logger.Info("Reconcile failed", "message", err.Error(), "retryInterval", "20s")
		return 20 * time.Second
	}

	if errors.Is(err, MasterBeElectingErr) || errors.Is(err, ManyMastersErr) || errors.Is(err, DeprecatedErr) {
		logger.Info("Reconcile failed", "message", err.Error(), "retryInterval", "1min")
		return 1 * time.Minute
	}

	logger.Error(err, "Reconcile failed", "retryInterval", "30s")
	return 30 * time.Second
}
