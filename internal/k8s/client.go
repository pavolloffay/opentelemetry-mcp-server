package k8s

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

var otelCollectorGVR = schema.GroupVersionResource{
	Group:    "opentelemetry.io",
	Version:  "v1beta1",
	Resource: "opentelemetrycollectors",
}

type Client struct {
	dynamic dynamic.Interface
}

type CollectorCustomResource struct {
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Version   string         `json:"version"`
	Mode      v1beta1.Mode   `json:"mode"`
	Config    v1beta1.Config `json:"config"`
}

// NewClient creates a new k8s client using default kubeconfig
func NewClient() (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{dynamic: dynamicClient}, nil
}

// ListCollectors lists all OpenTelemetryCollector CRs
// namespace="" for all namespaces
func (c *Client) ListCollectors(ctx context.Context, namespace string) ([]CollectorCustomResource, error) {
	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = c.dynamic.Resource(otelCollectorGVR).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynamic.Resource(otelCollectorGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list collectors: %w", err)
	}

	collectors := make([]CollectorCustomResource, 0, len(list.Items))
	for _, item := range list.Items {
		collector, err := parseCollector(&item)
		if err != nil {
			return nil, fmt.Errorf("failed to parse collector %s/%s: %w", item.GetNamespace(), item.GetName(), err)
		}
		collectors = append(collectors, *collector)
	}

	return collectors, nil
}

// GetCollector gets a specific OpenTelemetryCollector CR
func (c *Client) GetCollector(ctx context.Context, namespace, name string) (*CollectorCustomResource, error) {
	item, err := c.dynamic.Resource(otelCollectorGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get collector %s/%s: %w", namespace, name, err)
	}

	return parseCollector(item)
}

// parseCollector converts unstructured to CollectorCustomResource
func parseCollector(item *unstructured.Unstructured) (*CollectorCustomResource, error) {
	name := item.GetName()
	namespace := item.GetNamespace()

	version, _, _ := unstructured.NestedString(item.Object, "status", "version")
	modeStr, _, _ := unstructured.NestedString(item.Object, "spec", "mode")

	configMap, found, err := unstructured.NestedMap(item.Object, "spec", "config")
	if err != nil || !found {
		return nil, fmt.Errorf("config not found in collector %s/%s", namespace, name)
	}

	config, err := convertToConfig(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &CollectorCustomResource{
		Name:      name,
		Namespace: namespace,
		Version:   version,
		Mode:      v1beta1.Mode(modeStr),
		Config:    *config,
	}, nil
}

// convertToConfig converts map to v1beta1.Config
func convertToConfig(m map[string]interface{}) (*v1beta1.Config, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	var config v1beta1.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
