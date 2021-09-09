package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)


type obj interface {
	GetObjectKind() schema.ObjectKind
}

func NewClient[T obj](config *rest.Config) client[T] {
	return client[T] {
		dyn: dynamic.NewForConfigOrDie(config),
	}
}

type client[T obj] struct {
	dyn dynamic.Interface // TODO: don't depend on dynamic client
}

// gvr translates the T to a GVR, via GVK
// GVK->GVR translation is a terrible hack; we need something better.
func (c client[T]) gvr() schema.GroupVersionResource {
	// BOOM!
	// This panics because T is a nil *corev1.Pod; the runtime.Object interface is only satisfied by *corev1.Pod, not the instantiable corev1.Pod.
	// https://go.googlesource.com/proposal/+/refs/heads/master/design/43651-type-parameters.md#pointer-method-example
	// https://go.googlesource.com/proposal/+/refs/heads/master/design/43651-type-parameters.md#no-way-to-require-pointer-methods
	var t T
	gvk := t.GetObjectKind().GroupVersionKind()
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind) + "s", // HACK
	}
}

func (c client[T]) List(ctx context.Context, namespace string) ([]T, error) {
	ul, err := c.dyn.Resource(c.gvr()).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []T
	for _, u := range ul.Items {
		var t T
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(u.Object); err != nil { return nil, err }
		if err := json.NewDecoder(&buf).Decode(t); err != nil { return nil, err }
		out = append(out, t)
	}
	return out, nil
}

func (c client[T]) Get(ctx context.Context, namespace, name string) (T, error) {
	var t T
	u, err := c.dyn.Resource(c.gvr()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return t, err
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(u.Object); err != nil { return t, err }
	if err := json.NewDecoder(&buf).Decode(t); err != nil { return t, err }
	return t, nil
}

func (c client[T]) Create(ctx context.Context, namespace string, t T) error {
	m := map[string]interface{}{}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(t); err != nil { return err }
	if err := json.NewDecoder(&buf).Decode(&m); err != nil { return err }
	u := &unstructured.Unstructured{Object:m}
	_, err := c.dyn.Resource(c.gvr()).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
	return err
}

