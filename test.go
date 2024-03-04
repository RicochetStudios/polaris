package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	client "github.com/RicochetStudios/polaris/clientset/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	ctx := context.Background()

	// Get Kube Config.
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// Build the kubeconfig authenticate with it via kubectl
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	// Create the clientset.
	clientset, err := client.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// p, err := clientset.Polaris("default").Get(ctx, "polaris-00000001", metav1.GetOptions{})
	// if err != nil {
	// 	panic(err)
	// }

	// Get server from serverId using polaris interface
	p, err := clientset.Polaris("default").Get(ctx, "polaris-00000001", metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Server: %v\n", p)
}
