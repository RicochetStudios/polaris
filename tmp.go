package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	v1 "github.com/RicochetStudios/polaris/apis/v1"
	client "github.com/RicochetStudios/polaris/pkg/clientset"
	fakeclient "github.com/RicochetStudios/polaris/pkg/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var kubeconfig *string

type Client struct {
	Clientset client.Interface
}

func init() {
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
}

func main() {
	ctx := context.TODO()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	// Create the real client.
	clientset, err := client.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	client := Client{
		Clientset: clientset,
	}
	// Get the real Polaris.
	polaris, err := client.GetPolaris(ctx, "polaris-00000001")
	if err != nil {
		panic(err)
	}

	// Create the fake client.
	fakeClientset := fakeclient.NewSimpleClientset()
	if err != nil {
		panic(err)
	}
	fakeclient := Client{
		Clientset: fakeClientset,
	}
	// Get the fake Polaris.
	fakePolaris, err := fakeclient.GetPolaris(ctx, "fake")
	if err != nil {
		panic(err)
	}

	fmt.Print(polaris)
	fmt.Print(fakePolaris)
}

func (c Client) GetPolaris(ctx context.Context, name string) (*v1.Polaris, error) {
	p, err := c.Clientset.PolarisV1().Polarises("default").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return p, nil
}
