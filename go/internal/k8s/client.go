// Package k8s builds Kubernetes API clients and polls pod/metrics data.
package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Clients bundles the two clientsets the poller needs.
type Clients struct {
	// Core is the standard Kubernetes clientset, used to list pod
	// identity/status/restarts.
	Core kubernetes.Interface
	// Metrics is the metrics.k8s.io clientset, used to list pod
	// CPU/memory usage.
	Metrics metricsv.Interface
}

// BuildClients constructs the agent's Kubernetes clientsets.
//
// Purpose: builds Clients using in-cluster credentials if available,
// falling back to the local kubeconfig (KUBECONFIG, or
// ~/.kube/config) for development against a cluster like KIND.
// Params: none.
// Returns: the constructed Clients, or an error if no credentials
// could be found or either clientset failed to build.
func BuildClients() (*Clients, error) {
	restConfig, err := buildRestConfig(rest.InClusterConfig, loadKubeconfig)
	if err != nil {
		return nil, err
	}

	core, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building core clientset: %w", err)
	}

	metrics, err := metricsv.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("building metrics clientset: %w", err)
	}

	return &Clients{Core: core, Metrics: metrics}, nil
}

// buildRestConfig resolves a *rest.Config with a fallback ordering.
//
// Purpose: tries inCluster first, falling back to outOfCluster. Both
// loaders are injected so the fallback ordering can be unit tested
// without real cluster credentials or a kubeconfig file.
// Params:
//   - inCluster: loader tried first (e.g. rest.InClusterConfig).
//   - outOfCluster: loader tried if inCluster fails (e.g. kubeconfig).
//
// Returns: the first loader's config that succeeds, or an error
// combining both failures if both fail.
func buildRestConfig(inCluster, outOfCluster func() (*rest.Config, error)) (*rest.Config, error) {
	cfg, err := inCluster()
	if err == nil {
		return cfg, nil
	}

	cfg, kubeErr := outOfCluster()
	if kubeErr != nil {
		return nil, fmt.Errorf("no in-cluster config (%v) and failed to load kubeconfig (%w)", err, kubeErr)
	}

	return cfg, nil
}

// loadKubeconfig builds a *rest.Config from a local kubeconfig file.
//
// Purpose: loads a *rest.Config from KUBECONFIG, or ~/.kube/config
// if KUBECONFIG is unset.
// Params: none.
// Returns: the loaded config, or an error if the home directory or
// kubeconfig file couldn't be resolved/parsed.
func loadKubeconfig() (*rest.Config, error) {
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		path = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", path)
}
