package generic

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func TestNewClient(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	config := &rest.Config{}
	client := NewClient[*corev1.Pod](gvr, config)

	if client.gvr != gvr {
		t.Errorf("expected GVR %v, got %v", gvr, client.gvr)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: namespace,
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: namespace,
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, pod1, pod2)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	pods, err := client.List(ctx, namespace)
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

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, expectedPod)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	pod, err := client.Get(ctx, namespace, podName)
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

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	if err := client.Create(ctx, namespace, newPod); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify the pod was created
	created, err := client.Get(ctx, namespace, "new-pod")
	if err != nil {
		t.Fatalf("Failed to get created pod: %v", err)
	}

	if created.Name != "new-pod" {
		t.Errorf("expected pod name new-pod, got %s", created.Name)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	originalPod := &corev1.Pod{
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

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, originalPod)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	// Update the pod
	updatedPod := originalPod.DeepCopy()
	updatedPod.Spec.Containers[0].Image = "nginx:2.0"

	if err := client.Update(ctx, namespace, updatedPod); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify the update
	result, err := client.Get(ctx, namespace, "update-pod")
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

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete-pod",
			Namespace: namespace,
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, pod)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	// Delete the pod
	if err := client.Delete(ctx, namespace, "delete-pod"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	_, err := client.Get(ctx, namespace, "delete-pod")
	if err == nil {
		t.Error("expected pod to be deleted, but it still exists")
	}
}

func TestPatch(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "patch-pod",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test",
			},
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, pod)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	client := client[*corev1.Pod]{
		gvr: gvr,
		dyn: dynClient,
	}

	// Create a JSON patch
	patch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  "/metadata/labels/environment",
			"value": "production",
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Patch(ctx, namespace, "patch-pod", types.JSONPatchType, patchData); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	// Verify the patch
	result, err := client.Get(ctx, namespace, "patch-pod")
	if err != nil {
		t.Fatalf("Failed to get patched pod: %v", err)
	}

	if result.Labels["environment"] != "production" {
		t.Errorf("expected label environment=production, got %s", result.Labels["environment"])
	}
}

func TestInform(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	namespace := "test-namespace"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inform-pod",
			Namespace: namespace,
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, pod)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	// Create client with informer factory
	config := &rest.Config{}
	client := NewClient[*corev1.Pod](gvr, config)
	client.dyn = dynClient // Override with fake client for testing

	// Track events
	events := make(chan string, 10)

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			events <- "add"
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			events <- "update"
		},
		DeleteFunc: func(obj interface{}) {
			events <- "delete"
		},
	}

	// Start informer
	client.Start(ctx)
	client.Inform(ctx, handler)

	// Wait for initial sync
	select {
	case event := <-events:
		if event != "add" {
			t.Errorf("expected add event, got %s", event)
		}
	case <-ctx.Done():
		t.Fatal("context cancelled before receiving event")
	}
}

// Test with ConfigMap to verify generic behavior
func TestGenericWithConfigMap(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dynClient := fake.NewSimpleDynamicClient(scheme, cm)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	client := client[*corev1.ConfigMap]{
		gvr: gvr,
		dyn: dynClient,
	}

	// Test List
	configs, err := client.List(ctx, namespace)
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
