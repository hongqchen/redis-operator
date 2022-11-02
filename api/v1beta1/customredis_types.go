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

package v1beta1

import (
	"github.com/hongqchen/redis-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
type ClusterMode string

const (
	MasterSlave ClusterMode = "master-slave"
	Sentinel    ClusterMode = "sentinel"
	Cluster     ClusterMode = "cluster"
)

// CustomRedisSpec defines the desired state of CustomRedis
type CustomRedisSpec struct {
	// +kubebuilder:validation:Minimum=3
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Enum=master-slave;sentinel;cluster
	ClusterMode ClusterMode       `json:"clusterMode"`
	Templates   PodConfig         `json:"templates"`
	RedisConfig map[string]string `json:"redisConfig"`

	// +kubebuilder:default:=3
	SentinelNum  *int32                            `json:"sentinelNum,omitempty"`
	VolumeConfig *corev1.PersistentVolumeClaimSpec `json:"volumeConfig,omitempty"`
}

type PodConfig struct {
	// +kubebuilder:default:="busybox:1.28"
	InitImage string `json:"initImage"`

	// +kubebuilder:validation:MinLength=5
	Image string `json:"image"`

	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// UpdateStrategy appv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

// CustomRedisStatus defines the observed state of CustomRedis
type CustomRedisStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Phase util.CustomRedisPhase `json:"phase"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cr
// +kubebuilder:printcolumn:name="ClusterMode",type=string,JSONPath=`.spec.clusterMode`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// CustomRedis is the Schema for the customredis API
type CustomRedis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomRedisSpec   `json:"spec,omitempty"`
	Status CustomRedisStatus `json:"status,omitempty"`
}

func (crs *CustomRedisStatus) setDefault(cr *CustomRedis) bool {
	if cr.Status.Phase == "" {
		cr.Status.Phase = util.CustomRedisCreating
		return true
	}
	if cr.Status.Phase == util.CustomRedisRunning {
		cr.Status.Phase = util.CustomRedisScaling
		return true
	}

	return false
}

func (cr *CustomRedis) SetDefaultStatus() bool {
	return cr.Status.setDefault(cr)
}

//+kubebuilder:object:root=true

// CustomRedisList contains a list of CustomRedis
type CustomRedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomRedis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CustomRedis{}, &CustomRedisList{})
}
