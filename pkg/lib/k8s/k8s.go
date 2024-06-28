package k8s

import (
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Build a config for later client usage
// func Config() (*kubernetes.ClientSet, error) {
// 	kubeconfig := os.Getenv("KUBECONFIG")

// 	// Build the Kubernetes client configuration
// 	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return config, nil
// }

// Generate a clientset based off KUBECONFIG env var
// func ClientSet() (*kubernetes.Clientset, error) {

// 	// Create the Kubernetes client
// 	clientset, err := kubernetes.NewForConfig(config)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return clientset, nil
// }
