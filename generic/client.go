package generic

import (
	"bytes"
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)


func NewClient[T any](gvr schema.GroupVersionResource, config *rest.Config) client[T] {
	return client[T] {
		gvr: gvr,
		dyn: dynamic.NewForConfigOrDie(config),
	}
}

type client[T any] struct {
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

