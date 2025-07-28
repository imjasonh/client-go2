package generic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func TestPodClientGetLogs(t *testing.T) {
	mt := &mockTransport{
		responses: map[string]mockResponse{
			"GET /api/v1/namespaces/default/pods/test-pod/log": {
				statusCode: http.StatusOK,
				body:       "2024-01-01 12:00:00 Starting application...\n2024-01-01 12:00:01 Application ready",
			},
			"GET /api/v1/namespaces/default/pods/test-pod/log?container=sidecar&tailLines=10": {
				statusCode: http.StatusOK,
				body:       "Sidecar container logs here",
			},
		},
	}

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:      "http://localhost:8080",
			Transport: mt,
		},
	).PodClient("default")

	ctx := context.Background()

	t.Run("get logs without options", func(t *testing.T) {
		req := client.GetLogs("test-pod", nil)
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "2024-01-01 12:00:00 Starting application...\n2024-01-01 12:00:01 Application ready"
		if string(body) != expected {
			t.Errorf("expected logs %q, got %q", expected, string(body))
		}
	})

	t.Run("get logs with options", func(t *testing.T) {
		tailLines := int64(10)
		opts := &corev1.PodLogOptions{
			Container: "sidecar",
			TailLines: &tailLines,
		}
		req := client.GetLogs("test-pod", opts)
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "Sidecar container logs here"
		if string(body) != expected {
			t.Errorf("expected logs %q, got %q", expected, string(body))
		}
	})

	t.Run("stream logs", func(t *testing.T) {
		mt.responses["GET /api/v1/namespaces/default/pods/streaming-pod/log?follow=true"] = mockResponse{
			statusCode: http.StatusOK,
			body:       "Line 1\nLine 2\nLine 3",
		}

		follow := true
		opts := &corev1.PodLogOptions{
			Follow: follow,
		}
		req := client.GetLogs("streaming-pod", opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer func() {
			if err := stream.Close(); err != nil {
				t.Fatalf("failed to close stream: %v", err)
			}
		}()

		data, err := io.ReadAll(stream)
		if err != nil {
			t.Fatalf("failed to read stream: %v", err)
		}

		expected := "Line 1\nLine 2\nLine 3"
		if string(data) != expected {
			t.Errorf("expected stream data %q, got %q", expected, string(data))
		}
	})
}

func TestPodClientBind(t *testing.T) {
	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host: "http://localhost:8080",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"POST /api/v1/namespaces/default/pods/test-pod/binding": {
						statusCode: http.StatusCreated,
						body:       "",
					},
				},
			},
		},
	).PodClient("default")

	ctx := context.Background()

	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: "test-node",
		},
	}

	err := client.Bind(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPodClientEvict(t *testing.T) {
	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host: "http://localhost:8080",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"POST /api/v1/namespaces/default/pods/test-pod/eviction": {
						statusCode: http.StatusCreated,
						body:       "",
					},
				},
			},
		},
	).PodClient("default")

	ctx := context.Background()

	eviction := &policyv1beta1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	err := client.Evict(ctx, eviction)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPodClientProxyGet(t *testing.T) {
	mt := &mockTransport{
		responses: map[string]mockResponse{
			"GET /api/v1/namespaces/default/pods/http:test-pod:8080/proxy/healthz": {
				statusCode: http.StatusOK,
				body:       "ok",
			},
			"GET /api/v1/namespaces/default/pods/http:test-pod:8080/proxy/metrics?format=json": {
				statusCode: http.StatusOK,
				body:       `{"cpu": "100m", "memory": "256Mi"}`,
			},
		},
	}

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:      "http://localhost:8080",
			Transport: mt,
		},
	).PodClient("default")

	ctx := context.Background()

	t.Run("proxy get health", func(t *testing.T) {
		req := client.ProxyGet("http", "test-pod", "8080", "healthz", nil)
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(body) != "ok" {
			t.Errorf("expected response 'ok', got %q", string(body))
		}
	})

	t.Run("proxy get with params", func(t *testing.T) {
		params := map[string]string{
			"format": "json",
		}
		req := client.ProxyGet("http", "test-pod", "8080", "metrics", params)
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"cpu": "100m", "memory": "256Mi"}`
		if string(body) != expected {
			t.Errorf("expected response %q, got %q", expected, string(body))
		}
	})

	t.Run("proxy get without port", func(t *testing.T) {
		// Add response for empty port case
		mt.responses["GET /api/v1/namespaces/default/pods/test-pod/proxy/status"] = mockResponse{
			statusCode: http.StatusOK,
			body:       `{"status": "running"}`,
		}

		// When port is empty, JoinSchemeNamePort returns just the name
		req := client.ProxyGet("", "test-pod", "", "status", nil)
		body, err := req.DoRaw(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := `{"status": "running"}`
		if string(body) != expected {
			t.Errorf("expected response %q, got %q", expected, string(body))
		}
	})
}

func TestServiceClientProxyGet(t *testing.T) {
	client := NewClientGVR[*corev1.Service](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
		&rest.Config{
			Host: "http://localhost:8080",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/default/services/http:test-service:80/proxy/api/v1/health": {
						statusCode: http.StatusOK,
						body:       `{"status": "healthy"}`,
					},
				},
			},
		},
	).ServiceClient("default")

	ctx := context.Background()

	req := client.ProxyGet("http", "test-service", "80", "api/v1/health", nil)
	body, err := req.DoRaw(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"status": "healthy"}`
	if string(body) != expected {
		t.Errorf("expected response %q, got %q", expected, string(body))
	}
}

func TestPodClientMethod(t *testing.T) {
	// Create a generic client for pods
	genericPodClient := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{Host: "http://localhost:8080"},
	)

	// Get PodClient from the generic client
	podClient := genericPodClient.PodClient("default")

	// Verify the client has the correct GVR
	if podClient.client.gvr.Resource != "pods" {
		t.Errorf("expected resource 'pods', got %q", podClient.client.gvr.Resource)
	}
}

func TestPodClientMethodPanics(t *testing.T) {
	// Test that PodClient() panics on non-pod types
	genericCMClient := NewClientGVR[*corev1.ConfigMap](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		&rest.Config{Host: "http://localhost:8080"},
	)

	// This should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when calling PodClient() on ConfigMap client")
		} else {
			// Verify the panic message
			if msg, ok := r.(string); ok {
				if !strings.Contains(msg, "PodClient() can only be called on Client[*corev1.Pod]") {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}
	}()

	// This should panic
	_ = genericCMClient.PodClient("default")
}

func TestServiceClientMethod(t *testing.T) {
	// Test that ServiceClient() works on a service client
	genericSvcClient := NewClientGVR[*corev1.Service](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
		&rest.Config{Host: "http://localhost:8080"},
	)

	// Get ServiceClient from the generic client
	svcClient := genericSvcClient.ServiceClient("default")

	// Verify the client has the correct GVR
	if svcClient.client.gvr.Resource != "services" {
		t.Errorf("expected resource 'services', got %q", svcClient.client.gvr.Resource)
	}
}

func TestServiceClientMethodPanics(t *testing.T) {
	// Test that ServiceClient() panics on non-service types
	genericCMClient := NewClientGVR[*corev1.ConfigMap](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		&rest.Config{Host: "http://localhost:8080"},
	)

	// This should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when calling ServiceClient() on ConfigMap client")
		} else {
			// Verify the panic message
			if msg, ok := r.(string); ok {
				if !strings.Contains(msg, "ServiceClient() can only be called on Client[*corev1.Service]") {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}
	}()

	// This should panic
	_ = genericCMClient.ServiceClient("default")
}

func TestSubResource(t *testing.T) {
	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host: "http://localhost:8080",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/default/pods/test-pod/status": {
						statusCode: http.StatusOK,
						body:       `{"status": {"phase": "Running"}}`,
					},
				},
			},
		},
	)

	ctx := context.Background()

	// Test generic subresource access
	req := client.SubResource("default", "test-pod", "status")
	body, err := req.DoRaw(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(body), "Running") {
		t.Errorf("expected status to contain 'Running', got %q", string(body))
	}
}
