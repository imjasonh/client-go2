package generic

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// Lister provides type-safe wrapper around cache.GenericLister.
type Lister[T runtime.Object] struct {
	genericLister cache.GenericLister
}

// NamespaceLister provides type-safe wrapper around cache.GenericNamespaceLister.
type NamespaceLister[T runtime.Object] struct {
	genericNamespaceLister cache.GenericNamespaceLister
}

// NewLister creates a new type-safe lister from an informer.
func NewLister[T runtime.Object](informer cache.SharedIndexInformer, resource schema.GroupResource) *Lister[T] {
	return &Lister[T]{
		genericLister: cache.NewGenericLister(informer.GetIndexer(), resource),
	}
}

// List returns all objects that match the selector.
func (l *Lister[T]) List(selector labels.Selector) ([]T, error) {
	objs, err := l.genericLister.List(selector)
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(objs))
	for _, obj := range objs {
		if typed, ok := obj.(T); ok {
			result = append(result, typed)
		}
	}
	return result, nil
}

// Get returns the object with the given name.
// For namespaced resources, use ByNamespace(namespace).Get(name).
// For cluster-scoped resources, use Get(name) directly.
func (l *Lister[T]) Get(name string) (T, error) {
	var zero T
	obj, err := l.genericLister.Get(name)
	if err != nil {
		return zero, err
	}

	typed, ok := obj.(T)
	if !ok {
		return zero, fmt.Errorf("object is not of type %T", zero)
	}

	return typed, nil
}

// ByNamespace returns a namespace-scoped lister.
func (l *Lister[T]) ByNamespace(namespace string) *NamespaceLister[T] {
	return &NamespaceLister[T]{
		genericNamespaceLister: l.genericLister.ByNamespace(namespace),
	}
}

// List returns all objects in the namespace that match the selector.
func (nl *NamespaceLister[T]) List(selector labels.Selector) ([]T, error) {
	objs, err := nl.genericNamespaceLister.List(selector)
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(objs))
	for _, obj := range objs {
		if typed, ok := obj.(T); ok {
			result = append(result, typed)
		}
	}
	return result, nil
}

// Get returns the object with the given name in the namespace.
func (nl *NamespaceLister[T]) Get(name string) (T, error) {
	var zero T
	obj, err := nl.genericNamespaceLister.Get(name)
	if err != nil {
		return zero, err
	}

	typed, ok := obj.(T)
	if !ok {
		return zero, fmt.Errorf("object is not of type %T", zero)
	}

	return typed, nil
}
