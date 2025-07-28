package generic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

// mockTransport implements http.RoundTripper for testing
type mockTransport struct {
	responses map[string]mockResponse
}

type mockResponse struct {
	statusCode int
	body       string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.Method + " " + req.URL.Path
	if req.URL.RawQuery != "" {
		key += "?" + req.URL.RawQuery
	}
	if resp, ok := m.responses[key]; ok {
		return &http.Response{
			StatusCode: resp.statusCode,
			Body:       io.NopCloser(strings.NewReader(resp.body)),
			Header:     make(http.Header),
		}, nil
	}
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"not found","code":404}`)),
		Header:     make(http.Header),
	}, nil
}

func TestNewClientGVR(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	client := NewClientGVR[*corev1.Pod](
		gvr,
		&rest.Config{
			Host: "http://localhost",
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	if client.gvr != gvr {
		t.Errorf("expected GVR %v, got %v", gvr, client.gvr)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: namespace,
		},
	}
	pod2 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: namespace,
		},
	}

	podList := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodList",
			APIVersion: "v1",
		},
		Items: []corev1.Pod{*pod1, *pod2},
	}

	listJSON, _ := json.Marshal(podList)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/test-namespace/pods": {
						statusCode: 200,
						body:       string(listJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	pods, err := client.List(ctx, namespace, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(pods) != 2 {
		t.Errorf("expected 2 pods, got %d", len(pods))
	}

	podNames := make(map[string]bool)
	for _, pod := range pods {
		podNames[pod.Name] = true
	}

	if !podNames["pod1"] || !podNames["pod2"] {
		t.Error("expected pods not found")
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"
	podName := "test-pod"

	expectedPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}

	podJSON, _ := json.Marshal(expectedPod)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/test-namespace/pods/test-pod": {
						statusCode: 200,
						body:       string(podJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	pod, err := client.Get(ctx, namespace, podName, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if pod.Name != podName {
		t.Errorf("expected pod name %s, got %s", podName, pod.Name)
	}

	if len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Name != "nginx" {
		t.Error("pod spec doesn't match expected")
	}
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	newPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-pod",
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}

	podJSON, _ := json.Marshal(newPod)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"POST /api/v1/namespaces/test-namespace/pods": {
						statusCode: 201,
						body:       string(podJSON),
					},
					"GET /api/v1/namespaces/test-namespace/pods/new-pod": {
						statusCode: 200,
						body:       string(podJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	created, err := client.Create(ctx, namespace, newPod, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.Name != "new-pod" {
		t.Errorf("expected pod name new-pod, got %s", created.Name)
	}

	// Verify the pod was created
	fetched, err := client.Get(ctx, namespace, "new-pod", nil)
	if err != nil {
		t.Fatalf("Failed to get created pod: %v", err)
	}

	if fetched.Name != "new-pod" {
		t.Errorf("expected pod name new-pod, got %s", fetched.Name)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	originalPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "update-pod",
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.0",
				},
			},
		},
	}

	// Update the pod
	updatedPod := originalPod.DeepCopy()
	updatedPod.Spec.Containers[0].Image = "nginx:2.0"

	updatedJSON, _ := json.Marshal(updatedPod)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"PUT /api/v1/namespaces/test-namespace/pods/update-pod": {
						statusCode: 200,
						body:       string(updatedJSON),
					},
					"GET /api/v1/namespaces/test-namespace/pods/update-pod": {
						statusCode: 200,
						body:       string(updatedJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	updated, err := client.Update(ctx, namespace, updatedPod, nil)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Spec.Containers[0].Image != "nginx:2.0" {
		t.Errorf("expected image nginx:2.0, got %s", updated.Spec.Containers[0].Image)
	}

	// Verify the update
	result, err := client.Get(ctx, namespace, "update-pod", nil)
	if err != nil {
		t.Fatalf("Failed to get updated pod: %v", err)
	}

	if result.Spec.Containers[0].Image != "nginx:2.0" {
		t.Errorf("expected image nginx:2.0, got %s", result.Spec.Containers[0].Image)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"DELETE /api/v1/namespaces/test-namespace/pods/delete-pod": {
						statusCode: 200,
						body:       `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success"}`,
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Delete the pod
	if err := client.Delete(ctx, namespace, "delete-pod", nil); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestPatch(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "patch-pod",
			Namespace: namespace,
			Labels: map[string]string{
				"app":         "test",
				"environment": "production",
			},
		},
	}

	podJSON, _ := json.Marshal(pod)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"PATCH /api/v1/namespaces/test-namespace/pods/patch-pod": {
						statusCode: 200,
						body:       string(podJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Create a JSON patch
	patchData := []byte(`{"op": "add", "path": "/metadata/labels/environment", "value": "production"}`)
	if err := client.Patch(ctx, namespace, "patch-pod", types.JSONPatchType, patchData, nil); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}
}

// Test with ConfigMap to verify generic behavior
func TestGenericWithConfigMap(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	cmList := &corev1.ConfigMapList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMapList",
			APIVersion: "v1",
		},
		Items: []corev1.ConfigMap{*cm},
	}

	listJSON, _ := json.Marshal(cmList)

	client := NewClientGVR[*corev1.ConfigMap](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/test-namespace/configmaps": {
						statusCode: 200,
						body:       string(listJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Test List
	configs, err := client.List(ctx, namespace, nil)
	if err != nil {
		t.Fatalf("List ConfigMaps failed: %v", err)
	}

	if len(configs) != 1 {
		t.Errorf("expected 1 configmap, got %d", len(configs))
	}

	if configs[0].Data["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %s", configs[0].Data["key1"])
	}
}

// TestNewClientGVRCustomResource tests using NewClientGVR with a custom GVR
func TestNewClientGVRCustomResource(t *testing.T) {
	// Example: Using NewClientGVR for a custom resource that might not be in the scheme
	customGVR := schema.GroupVersionResource{
		Group:    "custom.io",
		Version:  "v1",
		Resource: "myresources",
	}

	config := &rest.Config{
		Host: "http://localhost",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Group: "custom.io", Version: "v1"},
			NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
		},
	}
	client := NewClientGVR[*corev1.Pod](customGVR, config) // Using Pod type as placeholder

	if client.gvr != customGVR {
		t.Errorf("expected custom GVR %v, got %v", customGVR, client.gvr)
	}
}

// TestListWithLabelSelector tests List with label selector
func TestListWithLabelSelector(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test",
			},
		},
	}
	// pod2 is defined to show what doesn't match the selector
	_ = &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "other",
			},
		},
	}

	podList := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodList",
			APIVersion: "v1",
		},
		Items: []corev1.Pod{*pod1}, // Only pod1 matches the selector
	}

	listJSON, _ := json.Marshal(podList)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/test-namespace/pods?labelSelector=app%3Dtest": {
						statusCode: 200,
						body:       string(listJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// List with label selector
	pods, err := client.List(ctx, namespace, &metav1.ListOptions{
		LabelSelector: "app=test",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(pods) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pods))
	}

	if pods[0].Name != "pod1" {
		t.Errorf("expected pod1, got %s", pods[0].Name)
	}
}

// TestListWithFieldSelector tests List with field selector
func TestListWithFieldSelector(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running-pod",
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	podList := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodList",
			APIVersion: "v1",
		},
		Items: []corev1.Pod{*pod1},
	}

	listJSON, _ := json.Marshal(podList)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"GET /api/v1/namespaces/test-namespace/pods?fieldSelector=status.phase%3DRunning": {
						statusCode: 200,
						body:       string(listJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// List with field selector
	pods, err := client.List(ctx, namespace, &metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(pods) != 1 {
		t.Errorf("expected 1 pod, got %d", len(pods))
	}

	if pods[0].Name != "running-pod" {
		t.Errorf("expected running-pod, got %s", pods[0].Name)
	}
}

// TestWatch tests the Watch method
func TestWatch(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	transport := &mockTransport{
		responses: map[string]mockResponse{
			"GET /api/v1/namespaces/test-namespace/pods?watch=true": {
				statusCode: 200,
				body:       "",
			},
		},
	}

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:      "http://localhost",
			APIPath:   "/api",
			Transport: transport,
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Test basic watch
	watcher, err := client.Watch(ctx, namespace, nil)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	if watcher == nil {
		t.Error("expected non-nil watcher")
	}

	// Test watch with label selector
	transport.responses["GET /api/v1/namespaces/test-namespace/pods?labelSelector=app%3Dtest&watch=true"] = mockResponse{
		statusCode: 200,
		body:       "",
	}

	watcher2, err := client.Watch(ctx, namespace, &metav1.ListOptions{
		LabelSelector: "app=test",
	})
	if err != nil {
		t.Fatalf("Watch with selector failed: %v", err)
	}
	if watcher2 == nil {
		t.Error("expected non-nil watcher with selector")
	}
}

// TestDeleteCollection tests the DeleteCollection method
func TestDeleteCollection(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	transport := &mockTransport{
		responses: map[string]mockResponse{
			"DELETE /api/v1/namespaces/test-namespace/pods": {
				statusCode: 200,
				body:       `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success"}`,
			},
		},
	}

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:      "http://localhost",
			APIPath:   "/api",
			Transport: transport,
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Test basic delete collection
	err := client.DeleteCollection(ctx, namespace, nil, nil)
	if err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}

	// Test delete collection with label selector
	transport.responses["DELETE /api/v1/namespaces/test-namespace/pods?labelSelector=app%3Dtest"] = mockResponse{
		statusCode: 200,
		body:       `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success"}`,
	}

	if err := client.DeleteCollection(ctx, namespace, nil, &metav1.ListOptions{
		LabelSelector: "app=test",
	}); err != nil {
		t.Fatalf("DeleteCollection with selector failed: %v", err)
	}
}

// TestUpdateStatus tests the UpdateStatus method
func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	updatedPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	podJSON, _ := json.Marshal(updatedPod)

	client := NewClientGVR[*corev1.Pod](
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		&rest.Config{
			Host:    "http://localhost",
			APIPath: "/api",
			Transport: &mockTransport{
				responses: map[string]mockResponse{
					"PUT /api/v1/namespaces/test-namespace/pods/test-pod/status": {
						statusCode: 200,
						body:       string(podJSON),
					},
				},
			},
			ContentConfig: rest.ContentConfig{
				GroupVersion:         &schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
			},
		},
	)

	// Update status
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	result, err := client.UpdateStatus(ctx, namespace, pod, nil)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if result.Name != "test-pod" {
		t.Errorf("expected pod name 'test-pod', got '%s'", result.Name)
	}

	if result.Status.Phase != corev1.PodRunning {
		t.Errorf("expected status phase Running, got %s", result.Status.Phase)
	}

	if len(result.Status.Conditions) != 1 || result.Status.Conditions[0].Type != corev1.PodReady {
		t.Error("expected Ready condition in status")
	}
}
