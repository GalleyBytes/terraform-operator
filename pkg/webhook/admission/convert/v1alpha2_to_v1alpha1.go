package convert

import (
	"encoding/json"
	"log"
	"time"

	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	tfv1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertV1alpha2ToV1alpha1(rawRequest []byte) ([]byte, runtime.Object, error) {
	start := time.Now()
	want := tfv1alpha1.Terraform{}
	have := tfv1alpha2.Terraform{}

	err := json.Unmarshal(rawRequest, &have)
	if err != nil {
		// The request to convert cannot be converted and will continue to fail because what is already
		// stored in kubernetes has been malformed or corrupted.

		// Perhaps the rawRequest accidentally has the wrong version stored. Can the request be
		// unmarshalled as the desired response?
		err := json.Unmarshal(rawRequest, &want)
		if err != nil {
			return []byte{}, &want, err
		}
		want.TypeMeta = metav1.TypeMeta{
			Kind:       "Terraform",
			APIVersion: tfv1alpha1.SchemeGroupVersion.String(),
		}
		rawResponse, _ := json.Marshal(want)
		return rawResponse, &want, nil

	}

	log.Printf("Should convert %s/%s from %s to %s\n", have.Namespace, have.Name, tfv1alpha2.SchemeGroupVersion, tfv1alpha1.SchemeGroupVersion)

	want.TypeMeta = metav1.TypeMeta{
		Kind:       "Terraform",
		APIVersion: tfv1alpha1.SchemeGroupVersion.String(),
	}
	want.ObjectMeta = have.ObjectMeta

	// Status is very important so TFO can continue where it left from last version
	want.Status.PodNamePrefix = have.Status.PodNamePrefix
	want.Status.Stages = []tfv1alpha1.Stage{
		{
			Generation:    have.Status.Stage.Generation,
			State:         tfv1alpha1.StageState(have.Status.Stage.State),
			PodType:       tfv1alpha1.PodType(have.Status.Stage.TaskType),
			Interruptible: tfv1alpha1.Interruptible(have.Status.Stage.Interruptible),
			Reason:        have.Status.Stage.Reason,
			StartTime:     have.Status.Stage.StartTime,
			StopTime:      have.Status.Stage.StopTime,
		},
	}
	want.Status.Phase = tfv1alpha1.StatusPhase(have.Status.Phase)
	want.Status.Exported = tfv1alpha1.ExportedFalse
	want.Status.LastCompletedGeneration = have.Status.LastCompletedGeneration
	rawResponse, _ := json.Marshal(want)
	log.Print("took ", time.Since(start).String(), " to complete conversion")
	return rawResponse, &want, nil
}
