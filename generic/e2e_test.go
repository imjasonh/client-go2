//go:build e2e
// +build e2e

package generic

import (
	"context"
	"encoding/json"
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

	for _, tt := range []struct {
		name        string
		inferFunc   func() (schema.GroupVersionResource, error)
		expectedGVR schema.GroupVersionResource
	}{{
		name: "Pod",
		inferFunc: func() (schema.GroupVersionResource, error) {
			return inferGVR[*corev1.Pod](config)
		},
		expectedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		},
	}, {
		name: "ConfigMap",
		inferFunc: func() (schema.GroupVersionResource, error) {
			return inferGVR[*corev1.ConfigMap](config)
		},
		expectedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "configmaps",
		},
	}, {
		name: "Service",
		inferFunc: func() (schema.GroupVersionResource, error) {
			return inferGVR[*corev1.Service](config)
		},
		expectedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "services",
		},
	}, {
		name: "Secret",
		inferFunc: func() (schema.GroupVersionResource, error) {
			return inferGVR[*corev1.Secret](config)
		},
		expectedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "secrets",
		},
	}, {
		name: "Namespace",
		inferFunc: func() (schema.GroupVersionResource, error) {
			return inferGVR[*corev1.Namespace](config)
		},
		expectedGVR: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "namespaces",
		},
	}} {
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
		pods, err := client.List(ctx, "default", nil)
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
		cms, err := client.List(ctx, "default", nil)
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
		svcs, err := client.List(ctx, "default", nil)
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

	// Track events
	events := make(chan string, 100)

	handler := InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, obj *corev1.ConfigMap) {
			namespace, name, _ := cache.SplitMetaNamespaceKey(key)
			if namespace == "default" {
				events <- "add:" + name
			}
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.ConfigMap) {
			namespace, name, _ := cache.SplitMetaNamespaceKey(key)
			if namespace == "default" {
				events <- "update:" + name
			}
		},
		OnDelete: func(key string, obj *corev1.ConfigMap) {
			namespace, name, _ := cache.SplitMetaNamespaceKey(key)
			if namespace == "default" {
				events <- "delete:" + name
			}
		},
		OnError: func(obj any, err error) {
			t.Fatalf("Informer error: %v for object %v", err, obj)
		},
	}

	client.Inform(ctx, handler, nil)

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

	created, err := client.Create(ctx, "default", testCM, nil)
	if err != nil {
		t.Fatalf("failed to create test configmap: %v", err)
	}
	testCM = created
	defer func() {
		// Clean up
		if err := client.Delete(ctx, "default", testCM.Name, nil); err != nil {
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
	updated, err := client.Update(ctx, "default", testCM, nil)
	if err != nil {
		t.Fatalf("failed to update test configmap: %v", err)
	}
	testCM = updated

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

// TestPodClientExpansionE2E tests PodClient expansion methods against a real cluster
func TestPodClientExpansionE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	ctx := context.Background()

	// Create a Pod client
	client, err := NewClient[*corev1.Pod](config)
	if err != nil {
		t.Fatalf("failed to create pod client: %v", err)
	}

	// List pods in kube-system to find a suitable test pod
	pods, err := client.List(ctx, "kube-system", &metav1.ListOptions{
		LabelSelector: "k8s-app=kube-dns",
	})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	if len(pods) == 0 {
		t.Fatal("no CoreDNS pods found in kube-system namespace")
	}

	// Use the first CoreDNS pod for testing
	testPod := pods[0]
	t.Logf("Using pod %s for expansion method tests", testPod.Name)

	// Get a namespace-scoped PodClient
	podClient := client.PodClient("kube-system")

	t.Run("GetLogs", func(t *testing.T) {
		// Test getting logs without options
		req := podClient.GetLogs(testPod.Name, nil)
		logs, err := req.DoRaw(ctx)
		if err != nil {
			// Some pods might not have logs yet, which is okay
			t.Logf("GetLogs without options returned error (this might be expected): %v", err)
		} else {
			t.Logf("Successfully retrieved logs (%d bytes)", len(logs))
		}

		// Test getting logs with options
		tailLines := int64(5)
		logOpts := &corev1.PodLogOptions{
			TailLines: &tailLines,
		}
		req = podClient.GetLogs(testPod.Name, logOpts)
		logs, err = req.DoRaw(ctx)
		if err != nil {
			t.Logf("GetLogs with TailLines returned error: %v", err)
		} else {
			t.Logf("Successfully retrieved last 5 lines of logs (%d bytes)", len(logs))
			// Count lines to verify we got at most 5
			lines := 0
			for _, c := range logs {
				if c == '\n' {
					lines++
				}
			}
			if lines > 5 {
				t.Errorf("expected at most 5 lines, got %d", lines)
			}
		}

		// Test getting logs from specific container if pod has multiple containers
		if len(testPod.Spec.Containers) > 0 {
			containerName := testPod.Spec.Containers[0].Name
			containerOpts := &corev1.PodLogOptions{
				Container: containerName,
				TailLines: &tailLines,
			}
			req = podClient.GetLogs(testPod.Name, containerOpts)
			logs, err = req.DoRaw(ctx)
			if err != nil {
				t.Logf("GetLogs for container %s returned error: %v", containerName, err)
			} else {
				t.Logf("Successfully retrieved logs from container %s (%d bytes)", containerName, len(logs))
			}
		}
	})

	t.Run("ProxyGet", func(t *testing.T) {
		// Most pods don't expose HTTP endpoints, so we expect this to fail
		// but we're testing that the method works correctly
		req := podClient.ProxyGet("http", testPod.Name, "8080", "healthz", nil)
		_, err := req.DoRaw(ctx)
		if err != nil {
			// Expected - most pods don't have HTTP endpoints
			t.Logf("ProxyGet returned expected error: %v", err)
		} else {
			t.Log("ProxyGet unexpectedly succeeded")
		}
	})

	t.Run("PodClient panic on wrong type", func(t *testing.T) {
		// Create a ConfigMap client and verify it panics when calling PodClient()
		cmClient, err := NewClient[*corev1.ConfigMap](config)
		if err != nil {
			t.Fatalf("failed to create configmap client: %v", err)
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when calling PodClient() on ConfigMap client")
			} else {
				t.Logf("Got expected panic: %v", r)
			}
		}()

		// This should panic
		_ = cmClient.PodClient("default")
	})
}

// TestServiceClientExpansionE2E tests ServiceClient expansion methods against a real cluster
func TestServiceClientExpansionE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	ctx := context.Background()

	// Create a Service client
	client, err := NewClient[*corev1.Service](config)
	if err != nil {
		t.Fatalf("failed to create service client: %v", err)
	}

	// Get the kubernetes service in default namespace
	svc, err := client.Get(ctx, "default", "kubernetes", nil)
	if err != nil {
		t.Fatalf("failed to get kubernetes service: %v", err)
	}

	// Get a namespace-scoped ServiceClient
	serviceClient := client.ServiceClient("default")

	t.Run("ProxyGet", func(t *testing.T) {
		// The kubernetes service exposes the API server
		// Try to access the /version endpoint through the proxy
		req := serviceClient.ProxyGet("https", svc.Name, "443", "version", nil)
		body, err := req.DoRaw(ctx)
		if err != nil {
			// This might fail due to auth/TLS issues which is okay for this test
			t.Logf("ProxyGet returned error (might be expected): %v", err)
		} else {
			t.Logf("Successfully proxied request to kubernetes service, got response: %s", string(body))
		}
	})

	t.Run("ServiceClient panic on wrong type", func(t *testing.T) {
		// Create a Pod client and verify it panics when calling ServiceClient()
		podClient, err := NewClient[*corev1.Pod](config)
		if err != nil {
			t.Fatalf("failed to create pod client: %v", err)
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when calling ServiceClient() on Pod client")
			} else {
				t.Logf("Got expected panic: %v", r)
			}
		}()

		// This should panic
		_ = podClient.ServiceClient("default")
	})
}

// TestSubResourceE2E tests the generic SubResource method against a real cluster
func TestSubResourceE2E(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}

	ctx := context.Background()

	// Create a Pod client
	client, err := NewClient[*corev1.Pod](config)
	if err != nil {
		t.Fatalf("failed to create pod client: %v", err)
	}

	// List pods in kube-system
	pods, err := client.List(ctx, "kube-system", &metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	if len(pods) == 0 {
		t.Fatal("no pods found in kube-system namespace")
	}

	testPod := pods[0]

	t.Run("Get pod status subresource", func(t *testing.T) {
		req := client.SubResource("kube-system", testPod.Name, "status")
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("failed to get pod status: %v", err)
		}

		// Verify we got a valid pod status
		var pod corev1.Pod
		if err := json.Unmarshal(body, &pod); err != nil {
			t.Fatalf("failed to decode pod status: %v", err)
		}

		if pod.Status.Phase == "" {
			t.Error("expected pod status to have a phase")
		} else {
			t.Logf("Pod %s is in phase: %s", testPod.Name, pod.Status.Phase)
		}

		t.Logf("Successfully retrieved pod status (%d bytes)", len(body))
	})
}
