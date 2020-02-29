package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type TerraformInterface interface {
	List(opts metav1.ListOptions) (*TerraformList, error)
	Get(name string, options metav1.GetOptions) (*Terraform, error)
	Create(*Terraform) (*Terraform, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
}

// TerraformClient used to generate client for client-go usage
// +k8s:deepcopy-gen=false
type TerraformClient struct {
	restClient rest.Interface
	ns         string
}

func NewForConfig(c *rest.Config) (*TerraformClient, error) {
	config := *c
	config.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.UnversionedRESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &TerraformClient{restClient: client}, nil
}

func (c *TerraformClient) Terraforms(namespace string) TerraformInterface {
	return &TerraformClient{
		restClient: c.restClient,
		ns:         namespace,
	}
}

func (c *TerraformClient) List(opts metav1.ListOptions) (*TerraformList, error) {
	result := TerraformList{}
	err := c.restClient.Get().Namespace(c.ns).Resource("terraforms").VersionedParams(&opts, scheme.ParameterCodec).Do().Into(&result)
	return &result, err
}

func (c *TerraformClient) Get(name string, opts metav1.GetOptions) (*Terraform, error) {
	result := Terraform{}
	err := c.restClient.Get().Namespace(c.ns).Resource("terraforms").Name(name).VersionedParams(&opts, scheme.ParameterCodec).Do().Into(&result)
	return &result, err
}

func (c *TerraformClient) Create(terraform *Terraform) (*Terraform, error) {
	result := Terraform{}
	err := c.restClient.Post().Namespace(c.ns).Resource("terraforms").Body(terraform).Do().Into(&result)
	return &result, err
}

func (c *TerraformClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.restClient.Get().Namespace(c.ns).Resource("terraforms").VersionedParams(&opts, scheme.ParameterCodec).Watch()
}
