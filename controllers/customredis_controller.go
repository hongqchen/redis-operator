/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/hongqchen/redis-operator/pkg/controller"
	"github.com/hongqchen/redis-operator/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	redisv1beta1 "github.com/hongqchen/redis-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CustomRedisReconciler reconciles a CustomRedis object
type CustomRedisReconciler struct {
	client.Client
	Logger logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=redis.hongqchen,resources=customredis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.hongqchen,resources=customredis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.hongqchen,resources=customredis/finalizers,verbs=update
// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CustomRedis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *CustomRedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// logger 添加 namespace/name 字段
	namespacedName := req.NamespacedName
	logger := r.Logger.WithValues("instance", namespacedName)

	cRedis := &redisv1beta1.CustomRedis{}
	if err := r.Get(ctx, namespacedName, cRedis); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling")

	// 首次创建，更新 status 为 creating
	if cRedis.SetDefaultStatus() {
		logger.V(2).Info("Setting status -- creating")
		if err := r.Status().Update(ctx, cRedis); err != nil {
			return ctrl.Result{}, err
		}
	}

	redisHandler := controller.NewRedisHandler(r.Client, logger)
	if requeue := redisHandler.Sync(cRedis); requeue > 0 {
		return ctrl.Result{RequeueAfter: requeue}, nil
	}

	logger.V(2).Info("Setting status -- running")
	if cRedis.Status.Phase != util.CustomRedisRunning {
		cRedis.Status.Phase = util.CustomRedisRunning
		if err := r.Status().Update(ctx, cRedis); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomRedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1beta1.CustomRedis{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&appv1.StatefulSet{}, builder.WithPredicates(util.AnnotationsOrGenerationChanged{})).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			OwnerType:    &redisv1beta1.CustomRedis{},
			IsController: false,
		}, builder.WithPredicates(util.PodDeleted{})).
		Complete(r)
}
