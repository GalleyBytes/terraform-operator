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
	"encoding/json"
	"fmt"
	"testing"
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
					TerraformModule: "https://github.com/cloudposse/terraform-example-module.git?ref=master",
				},
			}
			Expect(k8sClient.Create(ctx, &terraform)).Should(Succeed())

			terraformLookupKey := types.NamespacedName{Name: TerraformName, Namespace: TerraformNamespace}
			createdTerraform := &tfv1alpha1.Terraform{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, terraformLookupKey, createdTerraform)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(createdTerraform.Status.Phase).Should(Equal(""))

			job := batchv1.Job{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, terraformLookupKey, &job)
				return err == nil

			}, timeout, interval).Should(BeTrue())

			By("By checking that the Terraform phase updates when job has active pods")

			job.Status.Active = int32(1)
			Expect(k8sClient.Status().Update(ctx, &job)).Should(Succeed())

			Eventually(func() (string, error) {
				err := k8sClient.Get(ctx, terraformLookupKey, createdTerraform)
				if err != nil {
					return "", err
				}

				return string(createdTerraform.Status.Phase), nil
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

				return string(createdTerraform.Status.Phase), nil
			}, timeout, interval).Should(Equal("stopped"))

		})
	})
})

func TestGetParsedAddress(t *testing.T) {
	var err error
	// p, err = getParsedAddress("foo::git::http://foobar.com//boo/bar//bash?ref=a12994d&url=example.com/chke/diil")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("git::ssh://git@github.com/user/repo//foo/bar/file?ref=12345632&sdf=http://go.com/ok")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("http://foobar.com//boo/bar//bash?ref=a12994d&url=example.com/chke/diil")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("github.com/user/repo.git//boo/bar//bash")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("http://user:password@github.com/user/repo.git//boo/bar//bash")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("s3://tf.isaaguilar.com/index//bash?ref=a12994d&url=example.com/chke/diil")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("git@github.com:user/repo//my/favorite/file.txt?ref=12345632")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("path/to/my/local.txt")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("/my/abs/path.out")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("../../up/a/directory.tf")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("fee/fie/foe?ref=0.1.0")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("example.com/awesomecorp/consul/happycloud")
	// if err != nil {
	// 	t.Error(err)
	// }
	// p, err = getParsedAddress("github.com/isaaguilar/terraform-aws-multi-account-peering")
	// if err != nil {
	// 	t.Error(err)
	// }
	// scmdetecotor := scmDetector{hosts: []string{"github.com"}}
	var b []byte
	var p ParsedAddress
	var exampleScmType scmType = "bar"

	scmMap := map[string]scmType{
		"github.com": gitScmType,
		"foo.io":     exampleScmType,
	}
	p, err = getParsedAddress("github.com/hashicorp/example", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("git@github.com:hashicorp/example.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("https://github.com/hashicorp/example//path/to/a//abs/to/b//root/c?do=this&url=https://google.com/ohno/ok&ref=master", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("git::https://example.com/vpc.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("git::ssh://username@example.com/storage.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("git::username@example.com:storage.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("hg::http://example.com/vpc.hg", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("https://example.com/vpc-module.zip", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("s3::https://s3-eu-west-1.amazonaws.com/examplecorp-terraform-modules/vpc.zip", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("ssh://username@example.com/storage.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("username@example.com:storage.git", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	p, err = getParsedAddress("username@foo.io:myfavoriteuser/myfavoriterepo.goo?ref=7654321", "", false, scmMap)
	if err != nil {
		t.Error(err)
	}
	b, _ = json.MarshalIndent(p, "", " ")
	fmt.Println(string(b))

	// fmt.Printf("%+v", parsed)
}
