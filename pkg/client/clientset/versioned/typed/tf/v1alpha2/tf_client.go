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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha2

import (
	v1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	"github.com/isaaguilar/terraform-operator/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type TfV1alpha2Interface interface {
	RESTClient() rest.Interface
	TerraformsGetter
}

// TfV1alpha2Client is used to interact with features provided by the tf.isaaguilar.com group.
type TfV1alpha2Client struct {
	restClient rest.Interface
}

func (c *TfV1alpha2Client) Terraforms(namespace string) TerraformInterface {
	return newTerraforms(c, namespace)
}

// NewForConfig creates a new TfV1alpha2Client for the given config.
func NewForConfig(c *rest.Config) (*TfV1alpha2Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &TfV1alpha2Client{client}, nil
}

// NewForConfigOrDie creates a new TfV1alpha2Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *TfV1alpha2Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new TfV1alpha2Client for the given RESTClient.
func New(c rest.Interface) *TfV1alpha2Client {
	return &TfV1alpha2Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha2.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *TfV1alpha2Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}