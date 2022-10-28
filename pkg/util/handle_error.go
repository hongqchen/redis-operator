package util

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"strings"
	"time"
)

func ErrorHandle(logger logr.Logger, err error) time.Duration {
	if err == nil {
		return 0
	}

	// statefulset 未创建，入队，等待下一次 reconcile
	if apierror.IsNotFound(err) {
		return 2 * time.Second
	}

	// pod is being created
	if errors.Is(err, AllPodReadyErr) || errors.Is(err, NoMasterErr) {
		logger.Info(strings.ToTitle(err.Error()))
		return 20 * time.Second
	}

	if errors.Is(err, MasterBeElectingErr) || errors.Is(err, ManyMastersErr) {
		logger.Info(strings.ToTitle(err.Error()))
		return 1 * time.Minute
	}

	logger.Error(err, "")
	return 30 * time.Second
}
