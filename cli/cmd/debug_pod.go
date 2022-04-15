package cmd

import (
	"fmt"

	"github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generatePod(tf *v1alpha1.Terraform) *corev1.Pod {
	terraformVersion := tf.Spec.TerraformVersion
	if terraformVersion == "" {
		terraformVersion = "1.1.5"
	}
	generation := fmt.Sprint(tf.Generation)
	versionedName := tf.Status.PodNamePrefix + "-v" + generation
	generateName := versionedName + "-debug-"
	generationPath := "/home/tfo-runner/generations/" + generation
	envs := tf.Spec.Env
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_RUNNER",
			Value: "debug",
		},
		{
			Name:  "TFO_RESOURCE",
			Value: tf.Name,
		},
		{
			Name:  "TFO_NAMESPACE",
			Value: tf.Namespace,
		},
		{
			Name:  "TFO_GENERATION",
			Value: generation,
		},
		{
			Name:  "TFO_GENERATION_PATH",
			Value: generationPath,
		},
		{
			Name:  "TFO_MAIN_MODULE",
			Value: generationPath + "/main",
		},
		{
			Name:  "TFO_TERRAFORM_VERSION",
			Value: tf.Spec.TerraformVersion,
		},
	}...)

	volumes := []corev1.Volume{
		{
			Name: "tfohome",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: tf.Status.PodNamePrefix,
					ReadOnly:  false,
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tfohome",
			MountPath: "/home/tfo-runner",
			ReadOnly:  false,
		},
	}
	envs = append(envs, corev1.EnvVar{
		Name:  "TFO_ROOT_PATH",
		Value: "/home/tfo-runner",
	})

	optional := true
	xmode := int32(0775)
	volumes = append(volumes, corev1.Volume{
		Name: "gitaskpass",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: versionedName,
				Optional:   &optional,
				Items: []corev1.KeyToPath{
					{
						Key:  "gitAskpass",
						Path: "GIT_ASKPASS",
						Mode: &xmode,
					},
				},
			},
		},
	})
	volumeMounts = append(volumeMounts, []corev1.VolumeMount{
		{
			Name:      "gitaskpass",
			MountPath: "/git/askpass",
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "GIT_ASKPASS",
			Value: "/git/askpass/GIT_ASKPASS",
		},
	}...)

	annotations := tf.Spec.RunnerAnnotations
	envFrom := []corev1.EnvFromSource{}

	for _, c := range tf.Spec.Credentials {
		if c.AWSCredentials.KIAM != "" {
			annotations["iam.amazonaws.com/role"] = c.AWSCredentials.KIAM
		}
	}

	for _, c := range tf.Spec.Credentials {
		if (v1alpha1.SecretNameRef{}) != c.SecretNameRef {
			envFrom = append(envFrom, []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: c.SecretNameRef.Name,
						},
					},
				},
			}...)
		}
	}

	labels := make(map[string]string)
	if len(tf.Spec.RunnerLabels) > 0 {
		labels = tf.Spec.RunnerLabels
	}
	labels["terraforms.tf.isaaguilar.com/generation"] = generation
	labels["terraforms.tf.isaaguilar.com/resourceName"] = tf.Name
	labels["terraforms.tf.isaaguilar.com/podPrefix"] = tf.Status.PodNamePrefix
	labels["terraforms.tf.isaaguilar.com/terraformVersion"] = tf.Spec.TerraformVersion
	labels["app.kubernetes.io/name"] = "terraform-operator"
	labels["app.kubernetes.io/component"] = "terraform-operator-cli"
	labels["app.kubernetes.io/instance"] = "debug"
	labels["app.kubernetes.io/created-by"] = "cli"

	initContainers := []corev1.Container{}
	containers := []corev1.Container{}

	// Make sure to use the same uid for containers so the dir in the
	// PersistentVolume have the correct permissions for the user
	user := int64(0)
	group := int64(2000)
	runAsNonRoot := false
	privileged := true
	allowPrivilegeEscalation := true
	seLinuxOptions := corev1.SELinuxOptions{}
	securityContext := &corev1.SecurityContext{
		RunAsUser:                &user,
		RunAsGroup:               &group,
		RunAsNonRoot:             &runAsNonRoot,
		Privileged:               &privileged,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		SELinuxOptions:           &seLinuxOptions,
	}
	restartPolicy := corev1.RestartPolicyNever

	containers = append(containers, corev1.Container{
		SecurityContext: securityContext,
		Name:            "debug",
		Image:           "isaaguilar/tf-runner-v5beta1:" + terraformVersion,
		Command: []string{
			"/bin/sleep", "600",
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
		EnvFrom:         envFrom,
		Env:             envs,
		VolumeMounts:    volumeMounts,
	})

	podSecurityContext := corev1.PodSecurityContext{
		FSGroup: &group,
	}
	serviceAccount := tf.Spec.ServiceAccount
	if serviceAccount == "" {
		// By prefixing the service account with "tf-", IRSA roles can use wildcard
		// "tf-*" service account for AWS credentials.
		serviceAccount = "tf-" + versionedName
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
			Namespace:    tf.Namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			SecurityContext:    &podSecurityContext,
			ServiceAccountName: serviceAccount,
			RestartPolicy:      restartPolicy,
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            volumes,
		},
	}

	return pod
}
