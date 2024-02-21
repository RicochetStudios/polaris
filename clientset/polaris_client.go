package clientset

import (
	"context"

	polarisv1 "github.com/RicochetStudios/polaris/api/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// const apiPath = "/apis/polaris.ricochet/v1/polaris"

type PolarisInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*polarisv1.PolarisList, error)
	Get(ctx context.Context, name string, options metav1.GetOptions) (*polarisv1.Polaris, error)
	Create(ctx context.Context, polaris *polarisv1.Polaris) (*polarisv1.Polaris, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Update(ctx context.Context, polaris *polarisv1.Polaris) (*polarisv1.Polaris, error)
	Delete(ctx context.Context, name string, options metav1.DeleteOptions) (*polarisv1.Polaris, error)
}

type ExampleInterface interface {
	Polaris(namespace string) PolarisInterface
}

type polarisClient struct {
	restClient rest.Interface
	ns         string
}

type ExampleClient struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*ExampleClient, error) {
	config := *c
	config.ContentConfig.GroupVersion = &polarisv1.GroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &ExampleClient{restClient: client}, nil
}

func (c *ExampleClient) Polaris(namespace string) PolarisInterface {
	return &polarisClient{
		restClient: c.restClient,
		ns:         namespace,
	}
}

func (c *polarisClient) List(ctx context.Context, opts metav1.ListOptions) (*polarisv1.PolarisList, error) {
	result := polarisv1.PolarisList{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("polaris").
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("polaris").
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Create(ctx context.Context, polaris *polarisv1.Polaris) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Post().
		Namespace(c.ns).
		Resource("polaris").
		Body(polaris).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.restClient.
		Get().
		Namespace(c.ns).
		Resource("polaris").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch(ctx)
}

func (c *polarisClient) Update(ctx context.Context, polaris *polarisv1.Polaris) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Put().
		Namespace(c.ns).
		Resource("polaris").
		Name(polaris.Name).
		Body(polaris).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *polarisClient) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) (*polarisv1.Polaris, error) {
	result := polarisv1.Polaris{}
	err := c.restClient.
		Delete().
		Namespace(c.ns).
		Resource("polaris").
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}
