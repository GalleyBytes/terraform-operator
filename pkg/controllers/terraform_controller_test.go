/*
Copyright isaaguilar.

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
	"time"

	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Terraform controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		TerraformName      = "test-tfo"
		TerraformNamespace = "default"
		JobName            = "test-tfo"
		ServiceAccountName = "tf-test-tfo"
		Image              = "isaaguilar/tfops:0.13.5"
		ImagePullPolicy    = "Always"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When Creating Terraform", func() {
		It("Should update Terraform phase when jobs create pods", func() {
			By("By creating a new Terraform")
			ctx := context.Background()

			terraform := tfv1alpha1.Terraform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      TerraformName,
					Namespace: TerraformNamespace,
				},
				Spec: tfv1alpha1.TerraformSpec{
					TerraformModule: &tfv1alpha1.SrcOpts{
						Address: "https://github.com/cloudposse/terraform-example-module.git?ref=master",
					},
					ApplyOnCreate: true,
				},
			}
			Expect(k8sClient.Create(ctx, &terraform)).Should(Succeed())

			terraformLookupKey := types.NamespacedName{Name: TerraformName, Namespace: TerraformNamespace}
			createdTerraform := &tfv1alpha1.Terraform{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, terraformLookupKey, createdTerraform)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(createdTerraform.Status.Phase).Should(Equal(""))

			job := batchv1.Job{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, terraformLookupKey, &job)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("By checking that the Terraform phase updates when job has active pods")

			job.Status.Active = int32(1)
			Expect(k8sClient.Status().Update(ctx, &job)).Should(Succeed())

			Eventually(func() (string, error) {
				err := k8sClient.Get(ctx, terraformLookupKey, createdTerraform)
				if err != nil {
					return "", err
				}

				return createdTerraform.Status.Phase, nil
			}, timeout, interval).Should(Equal("running"))

			By("By checking that the Terraform phase updates when job changes to succeeded")

			job.Status.Active = int32(0)
			job.Status.Succeeded = int32(1)
			Expect(k8sClient.Status().Update(ctx, &job)).Should(Succeed())

			Eventually(func() (string, error) {
				err := k8sClient.Get(ctx, terraformLookupKey, createdTerraform)
				if err != nil {
					return "", err
				}

				return createdTerraform.Status.Phase, nil
			}, timeout, interval).Should(Equal("stopped"))

		})
	})
})
