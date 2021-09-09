package main

import (
	"bytes"
	"encoding/json"
	"context"
	"log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	ctx := context.Background()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.Fatalf("ClientConfig: %v", err)
	}

	// List pods in kube-system.
	pods, err := NewClient[pod](config).List(ctx, "kube-system")
	if err != nil { log.Fatal("listing pods:", err) }
	log.Println("PODS")
	for _, p := range pods {
		log.Println("-", p.Name)
	}

	// Create a ConfigMap, then list ConfigMaps.
	cmc := NewClient[cm](config)
	if err := cmc.Create(ctx, "kube-system",cm{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "foo-",
		},
		Data: map[string]string{
			"hello": "world",
		},
	}); err != nil {
		log.Fatal("creating configmap:", err)
	}
	cms, err := cmc.List(ctx, "kube-system")
	if err != nil { log.Fatal("listing configmaps:", err) }
	log.Println("CONFIGMAPS")
	for _, cm := range cms {
		log.Println("-", cm.Name)
	}
}

func NewClient[T obj](config *rest.Config) client[T] {
	return client[T] {
		dyn: dynamic.NewForConfigOrDie(config),
	}
}

type client[T obj] struct {
	dyn dynamic.Interface
}

func (c client[T]) List(ctx context.Context, namespace string) ([]T, error) {
	var t T
	ul, err := c.dyn.Resource(t.GVR()).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []T
	for _, u := range ul.Items {
		var t T
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(u.Object); err != nil { return nil, err }
		if err := json.NewDecoder(&buf).Decode(&t); err != nil { return nil, err }
		out = append(out, t)
	}
	return out, nil
}

func (c client[T]) Get(ctx context.Context, namespace, name string) (*T, error) {
	var t T
	u, err := c.dyn.Resource(t.GVR()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(u.Object); err != nil { return nil, err }
	if err := json.NewDecoder(&buf).Decode(&t); err != nil { return nil, err }
	return &t, nil
}

func (c client[T]) Create(ctx context.Context, namespace string, t T) error {
	m := map[string]interface{}{}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(t); err != nil { return err }
	if err := json.NewDecoder(&buf).Decode(&m); err != nil { return err }
	u := &unstructured.Unstructured{Object:m}
	_, err := c.dyn.Resource(t.GVR()).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
	return err
}

// THESE ARE HACKS
//
// Ideally, the Pod type (or any runtime.Object) would be able to give us its
// GVR directly, then the obj interface / type constraint would just be
// runtime.Object.
//
// Since it can't, we have to wrap runtime.Object and provide these per-type.
// This would be really nice to fix.

type pod corev1.Pod
func (pod pod) GetObjectKind() schema.ObjectKind { return pod.GetObjectKind() }
func (pod pod) DeepCopyObject() runtime.Object { return pod.DeepCopyObject() }
func (pod) GVR() schema.GroupVersionResource { 
	return schema.GroupVersionResource{
		Group: "",
		Version: "v1",
		Resource: "pods",
	}
}

type cm corev1.ConfigMap
func (cm cm) GetObjectKind() schema.ObjectKind { return cm.GetObjectKind() }
func (cm cm) DeepCopyObject() runtime.Object { return cm.DeepCopyObject() }
func (cm) GVR() schema.GroupVersionResource { 
	return schema.GroupVersionResource{
		Group: "",
		Version: "v1",
		Resource: "configmaps",
	}
}

type obj interface {
	runtime.Object

	GVR() schema.GroupVersionResource
}

