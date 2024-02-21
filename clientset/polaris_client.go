package clientset

import (
	"context"

	polarisv1 "github.com/RicochetStudios/polaris/api/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const apiPath = "/apis/polaris.ricochet/v1/polaris"

type PolarisInterface interface {
	List(opts metav1.ListOptions) (*polarisv1.PolarisList, error)
	Get(name string, options metav1.GetOptions) (*polarisv1.Polaris, error)
	Create(*polarisv1.Polaris) (*polarisv1.Polaris, error)
	Delete(name string, options metav1.DeleteOptions) (*polarisv1.Polaris, error)
}

type ExampleInterface interface {
	Polaris(ctx context.Context) PolarisInterface
}

type polarisClient struct {
	restClient rest.Interface
	ctx        context.Context
}

type ExampleClient struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*ExampleClient, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: polarisv1.GroupVersion.Group, Version: polarisv1.GroupVersion.Version}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &ExampleClient{restClient: client}, nil
}

func (c *ExampleClient) Polaris(ctx context.Context) PolarisInterface {
	return &polarisClient{
		restClient: c.restClient,
		ctx:        ctx,
	}
}

func (c *polarisClient) List(opts metav1.ListOptions) (*polarisv1.PolarisList, error) {
	result := polarisv1.PolarisList{}
	err := c.restClient.
		Get().
		AbsPath(apiPath).
		Do(c.ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Get(name string, opts metav1.GetOptions) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Get().
		AbsPath(apiPath).
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(c.ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Create(polaris *polarisv1.Polaris) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Post().
		AbsPath(apiPath).
		Body(polaris).
		Do(c.ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Update(polaris *polarisv1.Polaris) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Put().
		AbsPath(apiPath).
		Name(polaris.Name).
		Body(polaris).
		Do(c.ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Delete(name string, opts metav1.DeleteOptions) (*polarisv1.Polaris, error) {

	result := polarisv1.Polaris{}

	err := c.restClient.
		Delete().
		AbsPath(apiPath).
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(c.ctx).Into(&result)
	return &result, err
}
