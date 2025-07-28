package controller

import (
	"context"
	"fmt"

	"github.com/chainguard-dev/clog"
	"github.com/imjasonh/client-go2/generic"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// OwnerStrategy defines how to handle owner references for a resource type.
type OwnerStrategy[T runtime.Object, O runtime.Object] struct {
	// OwnerType is the GroupVersionKind of the owner resource
	OwnerType schema.GroupVersionKind

	// IsController indicates if we should only track controller references
	IsController bool
}

// EnqueueRequestForOwner returns a handler that enqueues the owner of an object.
func EnqueueRequestForOwner[T runtime.Object, O runtime.Object](
	ownerType schema.GroupVersionKind,
	isController bool,
) func(obj O) []string {
	return func(obj O) []string {
		// Get metadata from the object
		meta, err := getObjectMetaFromObject(obj)
		if err != nil {
			return nil
		}

		var requests []string
		for _, ref := range meta.GetOwnerReferences() {
			// Check if this matches our owner type
			if ref.APIVersion == ownerType.GroupVersion().String() && ref.Kind == ownerType.Kind {
				// Check controller flag if needed
				if isController && (ref.Controller == nil || !*ref.Controller) {
					continue
				}

				// Enqueue the owner
				key := fmt.Sprintf("%s/%s", meta.GetNamespace(), ref.Name)
				requests = append(requests, key)
			}
		}

		return requests
	}
}

// getObjectMetaFromObject extracts metav1.Object from a runtime.Object
func getObjectMetaFromObject(obj runtime.Object) (metav1.Object, error) {
	// Try to cast to metav1.Object
	if meta, ok := obj.(metav1.Object); ok {
		return meta, nil
	}

	// Try to get via accessor (handles wrapped objects)
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return accessor, nil
}

// WatchOwned configures the controller to watch resources of type O and enqueue
// their owners of type T when they change.
func (c *Controller[T]) WatchOwned(ctx context.Context, ownedClient generic.Client[runtime.Object], ownerGVK schema.GroupVersionKind, isController bool) error {
	// Create handler for owned resources
	handler := generic.InformerHandler[runtime.Object]{
		OnAdd: func(key string, obj runtime.Object) {
			c.enqueueOwners(ctx, obj, ownerGVK, isController)
		},
		OnUpdate: func(key string, oldObj, newObj runtime.Object) {
			// Enqueue owners from both old and new objects
			// This handles cases where ownership changes
			c.enqueueOwners(ctx, oldObj, ownerGVK, isController)
			c.enqueueOwners(ctx, newObj, ownerGVK, isController)
		},
		OnDelete: func(key string, obj runtime.Object) {
			c.enqueueOwners(ctx, obj, ownerGVK, isController)
		},
		OnError: func(obj any, err error) {
			clog.ErrorContext(ctx, "owned resource informer error", "error", err, "object", obj)
		},
	}

	opts := &generic.InformOptions{}
	if c.namespace != "" {
		opts.ListOptions.FieldSelector = fmt.Sprintf("metadata.namespace=%s", c.namespace)
	}

	// Start watching the owned resources
	go ownedClient.Inform(ctx, handler, opts)

	return nil
}

// enqueueOwners finds owners of the given object and enqueues them
func (c *Controller[T]) enqueueOwners(ctx context.Context, obj runtime.Object, ownerGVK schema.GroupVersionKind, isController bool) {
	meta, err := getObjectMetaFromObject(obj)
	if err != nil {
		clog.ErrorContext(ctx, "failed to get object metadata", "error", err)
		return
	}

	for _, ref := range meta.GetOwnerReferences() {
		// Check if this matches our owner type
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			continue
		}

		if gv.Group == ownerGVK.Group && gv.Version == ownerGVK.Version && ref.Kind == ownerGVK.Kind {
			// Check controller flag if needed
			if isController && (ref.Controller == nil || !*ref.Controller) {
				continue
			}

			// Build the key for the owner
			var key string
			if meta.GetNamespace() == "" {
				// Cluster-scoped owned resource
				key = ref.Name
			} else {
				key = fmt.Sprintf("%s/%s", meta.GetNamespace(), ref.Name)
			}

			ownedKey, _ := cache.MetaNamespaceKeyFunc(obj)
			clog.DebugContext(ctx, "enqueuing owner for owned resource change",
				"owner", key,
				"owned", ownedKey)
			c.queue.Add(key)
		}
	}
}

// GetOwnerReference creates an owner reference for the current object
func GetOwnerReference[T runtime.Object](owner T, scheme *runtime.Scheme) (metav1.OwnerReference, error) {
	meta, err := getObjectMetaFromObject(owner)
	if err != nil {
		return metav1.OwnerReference{}, err
	}

	gvks, _, err := scheme.ObjectKinds(owner)
	if err != nil {
		return metav1.OwnerReference{}, fmt.Errorf("could not get GVK for owner: %w", err)
	}

	if len(gvks) == 0 {
		return metav1.OwnerReference{}, fmt.Errorf("no GVK found for owner type")
	}

	gvk := gvks[0]

	return metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       meta.GetName(),
		UID:        meta.GetUID(),
	}, nil
}

// SetOwnerReference adds or updates an owner reference on the object
func SetOwnerReference[T runtime.Object](owned metav1.Object, owner T, scheme *runtime.Scheme, controller bool) error {
	ownerRef, err := GetOwnerReference(owner, scheme)
	if err != nil {
		return err
	}

	if controller {
		t := true
		ownerRef.Controller = &t
	}

	// Check if reference already exists
	refs := owned.GetOwnerReferences()
	for i, ref := range refs {
		if ref.UID == ownerRef.UID {
			// Update existing reference
			refs[i] = ownerRef
			owned.SetOwnerReferences(refs)
			return nil
		}
	}

	// Add new reference
	refs = append(refs, ownerRef)
	owned.SetOwnerReferences(refs)
	return nil
}

// RemoveOwnerReference removes an owner reference from the object
func RemoveOwnerReference[T runtime.Object](owned metav1.Object, owner T) error {
	meta, err := getObjectMetaFromObject(owner)
	if err != nil {
		return err
	}

	refs := owned.GetOwnerReferences()
	filtered := make([]metav1.OwnerReference, 0, len(refs))

	for _, ref := range refs {
		if ref.UID != meta.GetUID() {
			filtered = append(filtered, ref)
		}
	}

	owned.SetOwnerReferences(filtered)
	return nil
}

// IsOwnedBy checks if the object is owned by the given owner
func IsOwnedBy(owned metav1.Object, ownerUID string) bool {
	for _, ref := range owned.GetOwnerReferences() {
		if string(ref.UID) == ownerUID {
			return true
		}
	}
	return false
}

// GetControllerReference returns the controller owner reference if one exists
func GetControllerReference(owned metav1.Object) *metav1.OwnerReference {
	for _, ref := range owned.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return &ref
		}
	}
	return nil
}
