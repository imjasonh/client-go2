//go:build e2e
// +build e2e

package generic

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// TestInferGVRE2E tests GVR inference against a real Kubernetes cluster
func TestInferGVRE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	tests := []struct {
		name        string
		inferFunc   func() (schema.GroupVersionResource, error)
		expectedGVR schema.GroupVersionResource
	}{
		{
			name: "Pod",
			inferFunc: func() (schema.GroupVersionResource, error) {
				return inferGVR[*corev1.Pod](config)
			},
			expectedGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
		},
		{
			name: "ConfigMap",
			inferFunc: func() (schema.GroupVersionResource, error) {
				return inferGVR[*corev1.ConfigMap](config)
			},
			expectedGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
		},
		{
			name: "Service",
			inferFunc: func() (schema.GroupVersionResource, error) {
				return inferGVR[*corev1.Service](config)
			},
			expectedGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "services",
			},
		},
		{
			name: "Secret",
			inferFunc: func() (schema.GroupVersionResource, error) {
				return inferGVR[*corev1.Secret](config)
			},
			expectedGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "secrets",
			},
		},
		{
			name: "Namespace",
			inferFunc: func() (schema.GroupVersionResource, error) {
				return inferGVR[*corev1.Namespace](config)
			},
			expectedGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvr, err := tt.inferFunc()
			if err != nil {
				t.Fatalf("failed to infer GVR: %v", err)
			}

			if gvr != tt.expectedGVR {
				t.Errorf("expected GVR %v, got %v", tt.expectedGVR, gvr)
			}
		})
	}
}

// TestNewClientE2E tests client creation with inferred GVR against a real cluster
func TestNewClientE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	ctx := context.Background()

	t.Run("Pod client operations", func(t *testing.T) {
		client, err := NewClient[*corev1.Pod](config)
		if err != nil {
			t.Fatalf("failed to create pod client: %v", err)
		}

		// Try to list pods in default namespace
		pods, err := client.List(ctx, "default")
		if err != nil {
			t.Fatalf("failed to list pods: %v", err)
		}

		t.Logf("Successfully listed %d pods in default namespace", len(pods))
	})

	t.Run("ConfigMap client operations", func(t *testing.T) {
		client, err := NewClient[*corev1.ConfigMap](config)
		if err != nil {
			t.Fatalf("failed to create configmap client: %v", err)
		}

		// Try to list configmaps in default namespace
		cms, err := client.List(ctx, "default")
		if err != nil {
			t.Fatalf("failed to list configmaps: %v", err)
		}

		t.Logf("Successfully listed %d configmaps in default namespace", len(cms))
	})

	t.Run("Service client operations", func(t *testing.T) {
		client, err := NewClient[*corev1.Service](config)
		if err != nil {
			t.Fatalf("failed to create service client: %v", err)
		}

		// Try to list services in default namespace
		svcs, err := client.List(ctx, "default")
		if err != nil {
			t.Fatalf("failed to list services: %v", err)
		}

		t.Logf("Successfully listed %d services in default namespace", len(svcs))
	})
}

// TestInferGVRErrorCases tests error cases for GVR inference
func TestInferGVRErrorCases(t *testing.T) {
	config := &rest.Config{
		Host: "http://localhost:8080",
	}

	t.Run("Unregistered type", func(t *testing.T) {
		// This should fail because no scheme is registered for this type
		type UnregisteredType struct {
			*corev1.Pod
		}

		_, err := inferGVR[*UnregisteredType](config)
		if err == nil {
			t.Error("expected error for unregistered type, got nil")
		}
	})

}

// TestInformE2E tests informer functionality against a real cluster
func TestInformE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a ConfigMap client with inferred GVR
	client, err := NewClient[*corev1.ConfigMap](config)
	if err != nil {
		t.Fatalf("failed to create configmap client: %v", err)
	}

	// Start the informer
	client.Start(ctx)

	// Track events
	events := make(chan string, 100)

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// The dynamic informer returns unstructured objects
			meta, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			namespace, name, _ := cache.SplitMetaNamespaceKey(meta)
			if namespace == "default" {
				events <- "add:" + name
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			meta, err := cache.MetaNamespaceKeyFunc(newObj)
			if err != nil {
				return
			}
			namespace, name, _ := cache.SplitMetaNamespaceKey(meta)
			if namespace == "default" {
				events <- "update:" + name
			}
		},
		DeleteFunc: func(obj interface{}) {
			meta, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			namespace, name, _ := cache.SplitMetaNamespaceKey(meta)
			if namespace == "default" {
				events <- "delete:" + name
			}
		},
	}

	client.Inform(ctx, handler)

	// Create a test ConfigMap
	testCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-inform-" + time.Now().Format("20060102-150405"),
			Namespace: "default",
		},
		Data: map[string]string{
			"test": "data",
		},
	}

	if err := client.Create(ctx, "default", testCM); err != nil {
		t.Fatalf("failed to create test configmap: %v", err)
	}
	defer func() {
		// Clean up
		if err := client.Delete(ctx, "default", testCM.Name); err != nil {
			t.Logf("failed to delete test configmap: %v", err)
		}
	}()

	// Wait for add event for our specific ConfigMap
	deadline := time.After(10 * time.Second)
	foundAdd := false
	for !foundAdd {
		select {
		case event := <-events:
			t.Logf("Received event: %s", event)
			if event == "add:"+testCM.Name {
				foundAdd = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for add event")
		}
	}

	// Update the ConfigMap
	testCM.Data["test"] = "updated"
	if err := client.Update(ctx, "default", testCM); err != nil {
		t.Fatalf("failed to update test configmap: %v", err)
	}

	// Wait for update event
	deadline = time.After(10 * time.Second)
	foundUpdate := false
	for !foundUpdate {
		select {
		case event := <-events:
			t.Logf("Received event: %s", event)
			if event == "update:"+testCM.Name {
				foundUpdate = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for update event")
		}
	}
}
