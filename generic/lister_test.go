package generic

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func TestLister(t *testing.T) {
	// Create mock transport
	transport := &mockTransport{
		responses: make(map[string]mockResponse),
	}

	// Add discovery endpoints
	transport.responses["GET /api"] = mockResponse{
		statusCode: 200,
		body: `{
			"kind": "APIVersions",
			"versions": ["v1"],
			"serverAddressByClientCIDRs": [{"clientCIDR": "0.0.0.0/0", "serverAddress": "10.0.0.1:6443"}]
		}`,
	}
	transport.responses["GET /apis"] = mockResponse{
		statusCode: 200,
		body: `{
			"kind": "APIGroupList",
			"groups": []
		}`,
	}
	transport.responses["GET /api/v1"] = mockResponse{
		statusCode: 200,
		body: `{
			"kind": "APIResourceList",
			"groupVersion": "v1",
			"resources": [
				{"name": "configmaps", "singularName": "configmap", "namespaced": true, "kind": "ConfigMap", "verbs": ["create","delete","get","list","patch","update","watch"]}
			]
		}`,
	}

	// Setup mock response for initial list (without query params)
	transport.responses["GET /api/v1/configmaps"] = mockResponse{
		statusCode: 200,
		body: `{
			"kind": "ConfigMapList",
			"apiVersion": "v1",
			"metadata": {"resourceVersion": "1"},
			"items": [
				{
					"kind": "ConfigMap",
					"apiVersion": "v1",
					"metadata": {"name": "cm1", "namespace": "default", "resourceVersion": "1"},
					"data": {"key": "value1"}
				},
				{
					"kind": "ConfigMap", 
					"apiVersion": "v1",
					"metadata": {"name": "cm2", "namespace": "default", "labels": {"app": "test"}, "resourceVersion": "2"},
					"data": {"key": "value2"}
				},
				{
					"kind": "ConfigMap",
					"apiVersion": "v1",
					"metadata": {"name": "cm3", "namespace": "other", "resourceVersion": "3"},
					"data": {"key": "value3"}
				}
			]
		}`,
	}

	// Setup watch response - return empty to avoid decoding errors
	transport.responses["GET /api/v1/configmaps?watch=true"] = mockResponse{
		statusCode: 200,
		body:       "",
	}

	// Create client with mock transport
	config := &rest.Config{
		Host:      "http://test",
		Transport: transport,
	}

	// Use NewClientGVR to avoid discovery
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	client := NewClientGVR[*corev1.ConfigMap](gvr, config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track events
	var events []string
	handler := InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, cm *corev1.ConfigMap) {
			events = append(events, "add:"+key)
		},
		OnUpdate: func(key string, old, new *corev1.ConfigMap) {
			events = append(events, "update:"+key)
		},
		OnDelete: func(key string, cm *corev1.ConfigMap) {
			events = append(events, "delete:"+key)
		},
		OnError: func(obj any, err error) {
			t.Errorf("informer error: %v", err)
		},
	}

	// Start informer and get lister
	lister, err := client.Inform(ctx, handler, nil)
	if err != nil {
		t.Fatalf("failed to start informer: %v", err)
	}

	// Give time for initial sync
	time.Sleep(100 * time.Millisecond)

	// Test List all
	cms, err := lister.List(labels.Everything())
	if err != nil {
		t.Fatalf("failed to list all: %v", err)
	}
	if len(cms) != 3 {
		t.Errorf("expected 3 configmaps, got %d", len(cms))
	}

	// Test List with selector
	selector := labels.SelectorFromSet(labels.Set{"app": "test"})
	cms, err = lister.List(selector)
	if err != nil {
		t.Fatalf("failed to list with selector: %v", err)
	}
	if len(cms) != 1 {
		t.Errorf("expected 1 configmap with label app=test, got %d", len(cms))
	}

	// Test namespace lister
	nsLister := lister.ByNamespace("default")
	cms, err = nsLister.List(labels.Everything())
	if err != nil {
		t.Fatalf("failed to list in namespace: %v", err)
	}
	if len(cms) != 2 {
		t.Errorf("expected 2 configmaps in default namespace, got %d", len(cms))
	}

	// Test Get
	cm, err := nsLister.Get("cm1")
	if err != nil {
		t.Fatalf("failed to get cm1: %v", err)
	}
	if cm.Name != "cm1" {
		t.Errorf("expected name cm1, got %s", cm.Name)
	}

	// Test Get non-existent
	_, err = nsLister.Get("nonexistent")
	if err == nil {
		t.Error("expected error getting non-existent configmap")
	}
}
