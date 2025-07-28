package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
)

// Reconciler is the interface for reconciling objects of type T.
// Implementations should mutate the object in-place. The controller will
// automatically persist any changes to the object's status and finalizers
// after ReconcileKind returns successfully.
//
// Important rules:
//   - DO modify obj.Status to update status (will be persisted)
//   - DO modify obj.Finalizers to add/remove finalizers (will be persisted)
//   - DO NOT modify obj.Spec (changes will be ignored and logged)
//   - DO NOT modify obj.Metadata except for finalizers (changes will be ignored)
type Reconciler[T runtime.Object] interface {
	ReconcileKind(ctx context.Context, obj T) error
}

// ReconcilerFunc is an adapter to allow ordinary functions to be used as Reconcilers.
// If f is a function with the appropriate signature, ReconcilerFunc[T](f) is a
// Reconciler[T] that calls f.
type ReconcilerFunc[T runtime.Object] func(ctx context.Context, obj T) error

// ReconcileKind calls f(ctx, obj).
func (f ReconcilerFunc[T]) ReconcileKind(ctx context.Context, obj T) error {
	return f(ctx, obj)
}
