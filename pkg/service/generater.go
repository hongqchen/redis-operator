package service

import (
	"bytes"
	"fmt"
	"github.com/hongqchen/redis-operator/api/v1beta1"
	"github.com/hongqchen/redis-operator/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sort"
	"strconv"
	"strings"
)

var _ generater = (*generate)(nil)

type generater interface {
	createLabels(cRedis *v1beta1.CustomRedis) map[string]string
	createOwnerReference(cRedis *v1beta1.CustomRedis) []metav1.OwnerReference

	// master-slave
	configmap(cRedis *v1beta1.CustomRedis) *corev1.ConfigMap
	statefulset(cRedis *v1beta1.CustomRedis) *appv1.StatefulSet
	service(cRedis *v1beta1.CustomRedis) map[string]*corev1.Service

	// sentinel
	deployment(cRedis *v1beta1.CustomRedis) *appv1.Deployment
	configmapForSentinel(cRedis *v1beta1.CustomRedis) *corev1.ConfigMap
}

type generate struct{}

func newGenerate() *generate {
	return &generate{}
}

func (g *generate) getName(cRedis *v1beta1.CustomRedis) string {
	return cRedis.Name
}

func (g *generate) getNamespace(cRedis *v1beta1.CustomRedis) string {
	return cRedis.Namespace
}

func (g *generate) createOwnerReference(cRedis *v1beta1.CustomRedis) []metav1.OwnerReference {
	owner := []metav1.OwnerReference{
		*metav1.NewControllerRef(cRedis, schema.FromAPIVersionAndKind("redis.hongqchen/v1beta1", "CustomRedis")),
	}
	return owner
}

func (g *generate) createLabels(cRedis *v1beta1.CustomRedis) map[string]string {
	var labels = map[string]string{
		"hongqchen.com/controller": "custom-redis",
		"hongqchen.com/component":  "database",
		"hongqchen.com/name":       g.getName(cRedis),
	}
	return labels
}

func (g *generate) configmap(cRedis *v1beta1.CustomRedis) *corev1.ConfigMap {
	cm := cRedis.Spec.RedisConfig

	// 保证 requirepass 和 masterauth 成对出现
	if _, exists := cm["requirepass"]; exists {
		if _, exists := cm["masterauth"]; !exists {
			cm["masterauth"] = cm["requirepass"]
		}
	}

	// 将 yaml 格式转为 string
	var buffer bytes.Buffer

	keys := make([]string, 0, len(cm))
	for k := range cm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := cm[k]
		if len(v) == 0 {
			continue
		}
		buffer.WriteString(fmt.Sprintf("%s %s", k, v))
		buffer.WriteString("\n")
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            g.getName(cRedis),
			Namespace:       g.getNamespace(cRedis),
			OwnerReferences: g.createOwnerReference(cRedis),
			Labels:          g.createLabels(cRedis),
		},
		Data: map[string]string{
			util.RedisConfigFileName: buffer.String(),
		},
	}
}

func (g *generate) configmapForSentinel(cRedis *v1beta1.CustomRedis) *corev1.ConfigMap {
	configmapName := fmt.Sprintf("%s-%s", g.getName(cRedis), util.SentinelResourceSuffix)
	redisPort := cRedis.Spec.RedisConfig["port"]
	masterIP := "127.0.0.1"

	sentinelConf := []string{
		"sentinel down-after-milliseconds mymaster 30000",
		"sentinel failover-timeout mymaster 180000",
		"sentinel parallel-syncs mymaster 1",
	}
	sentinelConf = append(sentinelConf, fmt.Sprintf("sentinel monitor mymaster %s %s 2", masterIP, redisPort))

	authPass, exists := cRedis.Spec.RedisConfig["requirepass"]
	if exists {
		sentinelConf = append(sentinelConf, fmt.Sprintf("sentinel auth-pass mymaster %s", authPass))
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            configmapName,
			Namespace:       cRedis.Namespace,
			OwnerReferences: g.createOwnerReference(cRedis),
			Labels:          g.createLabels(cRedis),
		},
		Data: map[string]string{
			util.SentinelConfigFileName: strings.Join(sentinelConf, "\n"),
		},
	}
}

func (g *generate) statefulset(cRedis *v1beta1.CustomRedis) *appv1.StatefulSet {
	directory := cRedis.Spec.RedisConfig["dir"]
	pvcNamePrefix := "pvc"
	redisInstancePort, _ := strconv.ParseInt(cRedis.Spec.RedisConfig["port"], 10, 32)

	labels := g.createLabels(cRedis)
	delete(labels, "hongqchen.com/role")

	// default mount volumes
	volumes := []corev1.Volume{
		// redis.conf
		{
			Name: "volume-local-redisconf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cRedis.Name,
					},
					Items: []corev1.KeyToPath{
						{Key: util.RedisConfigFileName, Path: util.RedisConfigFileName},
					},
				},
			},
		},
		// mount host's timezone to pod
		{
			Name: "volume-local-time",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
	}

	var pvcVolumes []corev1.PersistentVolumeClaim
	if cRedis.Spec.VolumeConfig != nil {
		pvcVolumes = append(pvcVolumes, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcNamePrefix,
				Namespace: cRedis.Namespace,
			},
			Spec: *cRedis.Spec.VolumeConfig,
		})
	}

	volumesMount := []corev1.VolumeMount{
		{
			Name:      "volume-local-redisconf",
			ReadOnly:  false,
			MountPath: util.RedisConfigMountPath,
		},
		{
			Name:      "volume-local-time",
			ReadOnly:  true,
			MountPath: "/etc/localtime",
		},
	}

	if len(pvcVolumes) != 0 {
		// pvc exists
		volumesMount = append(volumesMount, corev1.VolumeMount{
			Name:      pvcNamePrefix,
			ReadOnly:  false,
			MountPath: directory,
		})
	} else {
		// pvc not exists, use emptyDir
		volumeName := "volume-redis-data"
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		volumesMount = append(volumesMount, corev1.VolumeMount{
			Name:      volumeName,
			ReadOnly:  false,
			MountPath: directory,
		})
	}

	// container info
	containers := []corev1.Container{
		{
			Name:    cRedis.Name,
			Image:   cRedis.Spec.Templates.Image,
			Command: []string{"redis-server"},
			Args:    []string{fmt.Sprintf("%s/%s", util.RedisConfigMountPath, util.RedisConfigFileName)},
			Ports: []corev1.ContainerPort{
				{
					Name:          "redis-port",
					ContainerPort: int32(redisInstancePort),
				},
			},
			Resources:       cRedis.Spec.Templates.Resources,
			VolumeMounts:    volumesMount,
			ImagePullPolicy: cRedis.Spec.Templates.ImagePullPolicy,
		},
	}

	return &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            g.getName(cRedis),
			Namespace:       g.getNamespace(cRedis),
			OwnerReferences: g.createOwnerReference(cRedis),
			Labels:          labels,
		},
		Spec: appv1.StatefulSetSpec{
			Replicas: cRedis.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			VolumeClaimTemplates: pvcVolumes,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: containers,
					Volumes:    volumes,
				},
			},
			UpdateStrategy: appv1.StatefulSetUpdateStrategy{
				Type: appv1.OnDeleteStatefulSetStrategyType,
			},
		},
	}
}

// service 配置渲染
// 不论何种模式，master、slave 均需创建 service
// sentinel，新增 sentinel 集群 service 创建
func (g *generate) service(cRedis *v1beta1.CustomRedis) map[string]*corev1.Service {
	name := cRedis.Name
	namespace := cRedis.Namespace
	//sentinelName := fmt.Sprintf("%s-%s", name, util.SentinelResourceSuffix)
	redisPort, _ := strconv.ParseInt(cRedis.Spec.RedisConfig["port"], 10, 32)
	labels := g.createLabels(cRedis)
	services := make(map[string]*corev1.Service, 3)

	// master
	// 拷贝 labels，添加 master role 键值对
	masterSelector := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		masterSelector[k] = v
	}
	masterSelector["redis.hongqchen/role"] = "master"

	masterService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-%s", name, "master"),
			Namespace:       namespace,
			Labels:          masterSelector,
			OwnerReferences: g.createOwnerReference(cRedis),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "redis-port",
					Port:     int32(redisPort),
					Protocol: corev1.ProtocolTCP,
				},
			},
			Selector: masterSelector,
		},
	}
	services[fmt.Sprintf("%s-%s", name, "master")] = masterService

	// slave
	slaveSelector := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		slaveSelector[k] = v
	}
	slaveSelector["redis.hongqchen/role"] = "slave"

	slaveService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-%s", name, "slave"),
			Namespace:       namespace,
			Labels:          slaveSelector,
			OwnerReferences: g.createOwnerReference(cRedis),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "redis-port",
					Port:     int32(redisPort),
					Protocol: corev1.ProtocolTCP,
				},
			},
			Selector: slaveSelector,
		},
	}
	services[fmt.Sprintf("%s-%s", name, "slave")] = slaveService

	// sentinel
	if cRedis.Spec.ClusterMode == v1beta1.Sentinel {
		sentinelSelector := make(map[string]string, len(labels)+1)
		for k, v := range labels {
			sentinelSelector[k] = v
		}
		sentinelSelector["redis.hongqchen/role"] = "sentinel"

		sentinelService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-%s", name, "sentinel"),
				Namespace:       namespace,
				Labels:          sentinelSelector,
				OwnerReferences: g.createOwnerReference(cRedis),
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "redis-port",
						Port:     int32(redisPort),
						Protocol: corev1.ProtocolTCP,
					},
				},
				Selector: sentinelSelector,
			},
		}

		services[fmt.Sprintf("%s-%s", name, "sentinel")] = sentinelService
	}

	return services
}

func (g *generate) deployment(cRedis *v1beta1.CustomRedis) *appv1.Deployment {
	name := fmt.Sprintf("%s-%s", cRedis.Name, util.SentinelResourceSuffix)
	directory := cRedis.Spec.RedisConfig["dir"]
	pvcNamePrefix := "pvc"
	namespace := cRedis.Namespace
	labels := g.createLabels(cRedis)
	delete(labels, "hongqchen.com/role")

	volumes := []corev1.Volume{
		{
			Name: "volume-sentinel-config-readonly",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Items: []corev1.KeyToPath{
						{Key: util.SentinelConfigFileName, Path: util.SentinelConfigFileName},
					},
				},
			},
		},
	}

	// pvcVolumes1111 := []corev1.PersistentVolumeClaim{}
	// if cr.Spec.VolumeConfig != nil {
	//	pvcVolumes = append(pvcVolumes, corev1.PersistentVolumeClaim{
	//		ObjectMeta: metav1.ObjectMeta{
	//			Name:      pvcNamePrefix,
	//			Namespace: cr.Namespace,
	//		},
	//		Spec: *cr.Spec.VolumeConfig,
	//	})
	// }

	if cRedis.Spec.VolumeConfig != nil {
		// pvc exists
		volumes = append(volumes, corev1.Volume{
			Name: "volume-sentinel-config-writable",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcNamePrefix,
				},
			},
		})
	} else {
		// pvc not exists, use emptyDir
		volumes = append(volumes, corev1.Volume{
			Name: "volume-sentinel-config-writable",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	containers := []corev1.Container{
		{
			Name:       util.SentinelResourceSuffix,
			Image:      cRedis.Spec.Templates.Image,
			Command:    []string{"redis-server"},
			Args:       []string{fmt.Sprintf("%s/%s", directory, util.SentinelConfigFileName), "--sentinel"},
			WorkingDir: "",
			Ports: []corev1.ContainerPort{
				{
					Name:          util.SentinelResourceSuffix,
					ContainerPort: util.SentinelPort,
					Protocol:      corev1.ProtocolTCP,
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "volume-sentinel-config-writable",
					MountPath: directory,
				},
			},
		},
	}

	initcontainers := []corev1.Container{
		{
			Name:  "prepare-sentinel-config",
			Image: cRedis.Spec.Templates.InitImage,
			Command: []string{
				"cp",
				fmt.Sprintf("%s/%s", util.RedisConfigMountPath, util.SentinelConfigFileName),
				fmt.Sprintf("%s/%s", directory, util.SentinelConfigFileName),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "volume-sentinel-config-readonly",
					MountPath: util.RedisConfigMountPath,
				},
				{
					Name:      "volume-sentinel-config-writable",
					MountPath: directory,
				},
			},
		},
	}

	return &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: g.createOwnerReference(cRedis),
			Labels:          labels,
		},
		Spec: appv1.DeploymentSpec{
			Replicas: cRedis.Spec.SentinelNum,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					InitContainers: initcontainers,
					Containers:     containers,
					Volumes:        volumes,
				},
			},
		},
	}
}
