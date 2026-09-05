package k8s

import (
	"errors"
	"testing"

	"k8s.io/client-go/rest"
)

func TestBuildRestConfig_PrefersInCluster(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return &rest.Config{Host: "https://in-cluster"}, nil
	}
	outOfCluster := func() (*rest.Config, error) {
		t.Fatal("outOfCluster loader should not be called when in-cluster succeeds")
		return nil, nil
	}

	cfg, err := buildRestConfig(inCluster, outOfCluster)
	if err != nil {
		t.Fatalf("buildRestConfig() error = %v", err)
	}
	if cfg.Host != "https://in-cluster" {
		t.Errorf("Host = %q, want %q", cfg.Host, "https://in-cluster")
	}
}

func TestBuildRestConfig_FallsBackToKubeconfig(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return nil, errors.New("not running in a pod")
	}
	outOfCluster := func() (*rest.Config, error) {
		return &rest.Config{Host: "https://kind-podsentinel"}, nil
	}

	cfg, err := buildRestConfig(inCluster, outOfCluster)
	if err != nil {
		t.Fatalf("buildRestConfig() error = %v", err)
	}
	if cfg.Host != "https://kind-podsentinel" {
		t.Errorf("Host = %q, want %q", cfg.Host, "https://kind-podsentinel")
	}
}

func TestBuildRestConfig_ErrorsWhenBothFail(t *testing.T) {
	inCluster := func() (*rest.Config, error) {
		return nil, errors.New("not running in a pod")
	}
	outOfCluster := func() (*rest.Config, error) {
		return nil, errors.New("no kubeconfig found")
	}

	_, err := buildRestConfig(inCluster, outOfCluster)
	if err == nil {
		t.Fatal("buildRestConfig() error = nil, want error when both loaders fail")
	}
}
