package generic

import (
	"bytes"
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ideally, this wouldn't need to take a GVR, and it could just be inferred
// from the runtime.Object type given.
//
// In practice, this doesn't seem to be straightforward. :(
//
// Things that implement runtime.Object tend to be pointer types (e.g.,
// *corev1.Pod), which means T is nil, and we can't call GetObjectKind() on it
// to start to guess at the GVR.
//
// It might be possible to use schemes to lookup the GVR for a given Go type
// (assuming it's been registered), which could let us get rid of this.
// In the meantime, taking a GVR removes ambiguity at the cost of verbosity.
// /shrug
func NewClient[T runtime.Object](gvr schema.GroupVersionResource, config *rest.Config) client[T] {
	return client[T] {
		gvr: gvr,
		dyn: dynamic.NewForConfigOrDie(config),
	}
}

type client[T runtime.Object] struct {
	gvr schema.GroupVersionResource
	dyn dynamic.Interface // TODO: don't depend on dynamic client
}

func (c client[T]) List(ctx context.Context, namespace string) ([]T, error) {
	ul, err := c.dyn.Resource(c.gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
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

func (c client[T]) Get(ctx context.Context, namespace, name string) (T, error) {
	var t T
	u, err := c.dyn.Resource(c.gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return t, err
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(u.Object); err != nil { return t, err }
	if err := json.NewDecoder(&buf).Decode(&t); err != nil { return t, err }
	return t, nil
}

func (c client[T]) Create(ctx context.Context, namespace string, t T) error {
	m := map[string]interface{}{}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(t); err != nil { return err }
	if err := json.NewDecoder(&buf).Decode(&m); err != nil { return err }
	u := &unstructured.Unstructured{Object:m}
	_, err := c.dyn.Resource(c.gvr).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
	return err
}

