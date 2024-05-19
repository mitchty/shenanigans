package local

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func wtf(format string, v ...interface{}) {
	format = fmt.Sprintf("debug: %s\n", format)
	_ = log.Output(2, fmt.Sprintf(format, v...))
}

// Create the namespace if it doesn't exist, simplifies the helm
// nonsense.
func ensureNamespace(clientset *kubernetes.Clientset, namespace string) error {
	// Check if the namespace already exists
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err == nil {
		// Namespace already exists
		return nil
	}

	// Create the namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func Cmd() error {
	kubeconfig := os.Getenv("KUBECONFIG")

	namespace := "cert-manager"
	releaseName := "cert-manager"
	repoName := "jetstack"
	repoURL := "https://charts.jetstack.io"
	chartName := "cert-manager"
	//	version := "v1.15.0" // Specify the version you want to install or upgrade to

	// Build the Kubernetes client configuration
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	// Create the Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	if err := ensureNamespace(clientset, namespace); err != nil {
		return err
	}

	settings := cli.New()
	settings.KubeConfig = kubeconfig
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), wtf); err != nil {
		return err
	}

	// Add the repository
	if err := addRepo(repoName, repoURL); err != nil {
		return err
	}

	// // List the chart URLs
	// chartURLs, err := listChartURLs(repoURL)
	// if err != nil {
	// 	log.Fatalf("Error listing chart URLs: %v", err)
	// }

	// for _, url := range chartURLs {
	// 	fmt.Println(url)
	// }

	// Install or upgrade cert-manager
	if err := installOrUpgradeCertManager(actionConfig, settings, repoURL, chartName, releaseName, namespace); err != nil {
		return err
	}

	fmt.Printf("cert-manager installed or upgraded successfully\n")
	return nil
}

func addRepo(repoName, repoURL string) error {
	repositoriesFile := os.ExpandEnv("$HOME/.config/helm/repositories.yaml")

	file, err := repo.LoadFile(repositoriesFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	entry := repo.Entry{
		Name: repoName,
		URL:  repoURL,
	}

	r, err := repo.NewChartRepository(&entry, getter.All(cli.New()))
	if err != nil {
		return err
	}

	_, err = r.DownloadIndexFile()
	if err != nil {
		return err
		// return fmt.Errorf("cannot reach repository: %s", repoURL)
	}
	//	fmt.Printf("%v\n", d)

	file.Update(&entry)
	return file.WriteFile(repositoriesFile, 0644)
}

// func listChartURLs(repoURL string) ([]string, error) {
// 	// Fetch the index file
// 	indexURL := fmt.Sprintf("%s/index.yaml", repoURL)
// 	resp, err := getter.All(cli.New()).Get(indexURL)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to fetch index file from %s: %v", indexURL, err)
// 	}
// 	defer resp.Close()

// 	// Load the index file
// 	indexFile, err := repo.LoadIndexFile(resp)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to load index file: %v", err)
// 	}

// 	var chartURLs []string
// 	for _, entries := range indexFile.Entries {
// 		for _, entry := range entries {
// 			for _, url := range entry.URLs {
// 				chartURLs = append(chartURLs, url)
// 			}
// 		}
// 	}

// 	return chartURLs, nil
// }

func installOrUpgradeCertManager(actionConfig *action.Configuration, settings *cli.EnvSettings, repoName, chartName, releaseName, namespace string) error {
	client := action.NewInstall(actionConfig)
	// client := action.NewInstall(actionConfig)
	client.Namespace = namespace
	//	client.Install = true
	// client.DryRun = true
	client.CreateNamespace = true
	fmt.Printf("repo is %s\n", repoName)
	client.RepoURL = repoName
	client.ReleaseName = releaseName
	//	client.ChartPathOptions.RepoURL = repoName
	client.Wait = true
	client.Timeout = 5 * time.Minute

	// chrt_path, err := client.LocateChart("https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.0.6/ingress-nginx-4.0.6.tgz", settings); if err != nil {
	//         panic(err)
	// }

	// myChart, err := loader.Load(chrt_path); if err != nil {
	//         panic(err)
	// }

	// chartPath, err := client.LocateChart(fmt.Sprintf("https://%s/%s", repoName, chartName), settings)
	// chartPath, err := client.ChartPathOptions.LocateChart("charts/cert-manager-v1.15.0.tgz", settings)
	// chartPath, err := client.ChartPathOptions.LocateChart(fmt.Sprintf("%s/%s", repoName, "charts/cert-manager-v1.15.0.tgz"), settings)
	// chartPath, err := client.ChartPathOptions.LocateChart(fmt.Sprintf("%s/%s", repoName, chartName), settings)
	// chartPath, err := client.ChartPathOptions.LocateChart("cert-manager", settings)
	chartPath, err := client.LocateChart("cert-manager", settings)
	if err != nil {
		return err
	}

	cmChart, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	vals := map[string]interface{}{
		// "installCRDs": true, // Automatically install CRDs
		// "crds.enabled": true, // Automatically install CRDs
	}

	// release, err := client.Run(releaseName, cmChart, vals)
	release, err := client.Run(cmChart, vals)
	if err != nil {
		return err
	}

	fmt.Printf("Release: %s\n", release.Name)
	return nil
}
