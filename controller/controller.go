package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/chainguard-dev/clog"
	"github.com/imjasonh/client-go2/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
)

// Options configures a Controller.
type Options[T runtime.Object] struct {
	// Namespace limits the controller to a specific namespace.
	// If empty, the controller watches all namespaces.
	Namespace string

	// Concurrency is the number of concurrent reconcilers.
	// Defaults to 1 if not set.
	Concurrency int

	// DeepCopyFunc is a custom deep copy function for objects.
	// If not provided, JSON round-trip is used.
	DeepCopyFunc func(T) T

	// Queue is a custom workqueue for the controller.
	// If not provided, a default rate-limiting queue is used.
	Queue workqueue.TypedRateLimitingInterface[string]

	// OwnedTypes is a list of resource types owned by the main resource type.
	// When owned resources change, the controller will reconcile their owners.
	OwnedTypes []OwnedType
}

// OwnedType represents a type that is owned by the main resource
type OwnedType struct {
	// Client is used to watch the owned resources
	Client generic.Client[runtime.Object]
	// OwnerGVK is the GroupVersionKind of the owner resource
	OwnerGVK schema.GroupVersionKind
	// IsController indicates if we should only track controller references
	IsController bool
}

// Controller manages the reconciliation loop for resources of type T.
type Controller[T runtime.Object] struct {
	client       generic.Client[T]
	reconciler   Reconciler[T]
	queue        workqueue.TypedRateLimitingInterface[string]
	namespace    string
	concurrency  int
	deepCopyFunc func(T) T
	ownedTypes   []OwnedType
	ownedListers map[schema.GroupVersionKind]*generic.Lister[runtime.Object]
}

// New creates a new Controller with the given client, reconciler, and options.
func New[T runtime.Object](client generic.Client[T], reconciler Reconciler[T], opts *Options[T]) *Controller[T] {
	// Apply defaults
	if opts == nil {
		opts = &Options[T]{}
	}
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Queue == nil {
		opts.Queue = workqueue.NewTypedRateLimitingQueue[string](workqueue.DefaultTypedControllerRateLimiter[string]())
	}

	return &Controller[T]{
		client:       client,
		reconciler:   reconciler,
		queue:        opts.Queue,
		namespace:    opts.Namespace,
		concurrency:  opts.Concurrency,
		ownedTypes:   opts.OwnedTypes,
		deepCopyFunc: opts.DeepCopyFunc,
		ownedListers: make(map[schema.GroupVersionKind]*generic.Lister[runtime.Object]),
	}
}

// Run starts the controller and blocks until the context is canceled.
func (c *Controller[T]) Run(ctx context.Context) error {
	defer c.queue.ShutDown()

	clog.InfoContext(ctx, "starting controller", "concurrency", c.concurrency)

	// Start the informer
	handler := generic.InformerHandler[T]{
		OnAdd: func(key string, obj T) {
			clog.DebugContext(ctx, "resource added", "key", key)
			c.queue.Add(key)
		},
		OnUpdate: func(key string, oldObj, newObj T) {
			clog.DebugContext(ctx, "resource updated", "key", key)
			c.queue.Add(key)
		},
		OnDelete: func(key string, obj T) {
			clog.DebugContext(ctx, "resource deleted", "key", key)
			c.queue.Add(key)
		},
		OnError: func(obj any, err error) {
			clog.ErrorContext(ctx, "informer error", "error", err, "object", obj)
		},
	}

	opts := &generic.InformOptions{}
	if c.namespace != "" {
		opts.ListOptions.FieldSelector = fmt.Sprintf("metadata.namespace=%s", c.namespace)
	}

	// Start informer in background
	go func() {
		if _, err := c.client.Inform(ctx, handler, opts); err != nil {
			clog.ErrorContext(ctx, "failed to start informer", "error", err)
		}
	}()

	// Start watching owned resources
	for _, owned := range c.ownedTypes {
		lister, err := WatchOwned(ctx, c, owned.Client, owned.IsController)
		if err != nil {
			return fmt.Errorf("failed to watch owned resources: %w", err)
		}
		c.ownedListers[owned.OwnerGVK] = lister
	}

	// Wait for cache sync
	clog.InfoContext(ctx, "waiting for cache sync")
	time.Sleep(time.Second) // Simple wait for now

	// Start workers
	for i := 0; i < c.concurrency; i++ {
		go c.runWorker(ctx)
	}

	<-ctx.Done()
	clog.InfoContext(ctx, "shutting down controller")
	return nil
}

// runWorker processes items from the queue.
func (c *Controller[T]) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// processNextItem processes one item from the queue.
func (c *Controller[T]) processNextItem(ctx context.Context) bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	if err := c.processItem(ctx, key); err != nil {
		c.handleProcessError(ctx, key, err)
		return true
	}

	clog.DebugContext(ctx, "successfully processed item", "key", key)
	c.queue.Forget(key)
	return true
}

// processItem fetches the object and calls the reconciler.
func (c *Controller[T]) processItem(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid key format: %w", err)
	}

	// Fetch current object
	current, err := c.client.Get(ctx, namespace, name, nil)
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}

	// Deep copy to preserve original for comparison
	original := c.deepCopy(current)

	// Call user's reconciler - they modify 'current' in place
	if err := c.reconciler.Reconcile(ctx, current); err != nil {
		// Don't update if reconciler returned error
		return err
	}

	// Update the object if needed
	return c.updateIfNeeded(ctx, original, current)
}

// updateIfNeeded compares the original and current objects and updates if necessary.
func (c *Controller[T]) updateIfNeeded(ctx context.Context, original, current T) error {
	// Extract metadata for both objects
	origMeta := c.getObjectMeta(original)
	currMeta := c.getObjectMeta(current)

	if origMeta == nil || currMeta == nil {
		return fmt.Errorf("failed to get object metadata")
	}

	// Check what changed
	specChanged := !c.equalSpecs(original, current)
	statusChanged := !c.equalStatus(original, current)
	finalizersChanged := !reflect.DeepEqual(origMeta.Finalizers, currMeta.Finalizers)
	annotationsChanged := !reflect.DeepEqual(origMeta.Annotations, currMeta.Annotations)
	labelsChanged := !reflect.DeepEqual(origMeta.Labels, currMeta.Labels)

	clog.DebugContext(ctx, "comparing objects",
		"namespace", currMeta.Namespace,
		"name", currMeta.Name,
		"origAnnotations", origMeta.Annotations,
		"currAnnotations", currMeta.Annotations)

	// Warn if spec changed (not allowed)
	if specChanged {
		clog.WarnContext(ctx, "spec changes ignored in Reconcile",
			"namespace", origMeta.Namespace,
			"name", origMeta.Name)
	}

	// Update metadata (finalizers, annotations, labels) if changed
	metadataChanged := finalizersChanged || annotationsChanged || labelsChanged
	if metadataChanged {
		clog.DebugContext(ctx, "metadata changed",
			"namespace", currMeta.Namespace,
			"name", currMeta.Name,
			"finalizersChanged", finalizersChanged,
			"annotationsChanged", annotationsChanged,
			"labelsChanged", labelsChanged)
		if err := c.updateMetadataWithRetry(ctx, original, current); err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
		// Refresh original for status update
		original = current
	}

	// Update status if changed
	if statusChanged {
		if err := c.updateStatusWithRetry(ctx, original, current); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
	}

	return nil
}

// updateMetadataWithRetry updates metadata (finalizers, annotations, labels) with conflict retry.
func (c *Controller[T]) updateMetadataWithRetry(ctx context.Context, original, current T) error {
	currMeta := c.getObjectMeta(current)
	if currMeta == nil {
		return fmt.Errorf("no metadata")
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get latest version
		latest, err := c.client.Get(ctx, currMeta.Namespace, currMeta.Name, nil)
		if err != nil {
			return err
		}

		// Copy metadata from current to latest
		latestMeta := c.getObjectMeta(latest)
		if latestMeta == nil {
			return fmt.Errorf("no metadata in latest")
		}

		// Copy finalizers
		latestMeta.Finalizers = currMeta.Finalizers

		// Copy annotations (ensure map exists)
		if currMeta.Annotations != nil {
			if latestMeta.Annotations == nil {
				latestMeta.Annotations = make(map[string]string)
			}
			for k, v := range currMeta.Annotations {
				latestMeta.Annotations[k] = v
			}
		}

		// Copy labels (ensure map exists)
		if currMeta.Labels != nil {
			if latestMeta.Labels == nil {
				latestMeta.Labels = make(map[string]string)
			}
			for k, v := range currMeta.Labels {
				latestMeta.Labels[k] = v
			}
		}

		// Update the object
		updated, err := c.client.Update(ctx, currMeta.Namespace, latest, nil)
		if err == nil {
			clog.DebugContext(ctx, "successfully updated metadata",
				"namespace", currMeta.Namespace,
				"name", currMeta.Name,
				"resourceVersion", c.getObjectMeta(updated).ResourceVersion)
		}
		return err
	})
}

// updateStatusWithRetry updates status with conflict retry.
func (c *Controller[T]) updateStatusWithRetry(ctx context.Context, original, current T) error {
	currMeta := c.getObjectMeta(current)
	if currMeta == nil {
		return fmt.Errorf("no metadata")
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get latest version
		latest, err := c.client.Get(ctx, currMeta.Namespace, currMeta.Name, nil)
		if err != nil {
			return err
		}

		// Copy status from current to latest
		if err := c.copyStatus(current, latest); err != nil {
			return fmt.Errorf("failed to copy status: %w", err)
		}

		// Update status
		_, err = c.client.UpdateStatus(ctx, currMeta.Namespace, latest, nil)
		return err
	})
}

// handleProcessError handles errors from processItem.
func (c *Controller[T]) handleProcessError(ctx context.Context, key string, err error) {
	// Check for permanent error
	if IsPermanentError(err) {
		clog.ErrorContext(ctx, "permanent error, not retrying", "key", key, "error", err)
		c.queue.Forget(key)
		return
	}

	// Check for requeue immediately
	if errors.Is(err, &requeueImmediately{}) {
		clog.DebugContext(ctx, "requeueing immediately", "key", key)
		c.queue.AddRateLimited(key)
		return
	}

	// Check for requeue after
	if duration := GetRequeueDuration(err); duration > 0 {
		clog.DebugContext(ctx, "requeueing after duration", "key", key, "duration", duration)
		c.queue.AddAfter(key, duration)
		return
	}

	// Default: requeue with rate limiting
	if c.queue.NumRequeues(key) < 10 { // TODO: make configurable
		clog.ErrorContext(ctx, "error processing item, requeueing", "key", key, "error", err)
		c.queue.AddRateLimited(key)
	} else {
		clog.ErrorContext(ctx, "max retries exceeded, dropping item", "key", key, "error", err)
		c.queue.Forget(key)
	}
}

// Helper methods for object manipulation

func (c *Controller[T]) deepCopy(obj T) T {
	if c.deepCopyFunc != nil {
		return c.deepCopyFunc(obj)
	}
	// Fallback to JSON round-trip (not efficient but works)
	data, _ := json.Marshal(obj)
	var copy T
	_ = json.Unmarshal(data, &copy)
	return copy
}

func (c *Controller[T]) getObjectMeta(obj T) *metav1.ObjectMeta {
	// Use reflection to get ObjectMeta
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	metaField := v.FieldByName("ObjectMeta")
	if !metaField.IsValid() {
		return nil
	}

	// Get a pointer to the ObjectMeta field to allow in-place modification
	if metaField.CanAddr() {
		metaPtr := metaField.Addr().Interface().(*metav1.ObjectMeta)
		return metaPtr
	}

	// Fallback: return a copy (won't allow in-place modification)
	meta, ok := metaField.Interface().(metav1.ObjectMeta)
	if !ok {
		return nil
	}
	return &meta
}

func (c *Controller[T]) equalSpecs(a, b T) bool {
	// Compare spec fields
	aSpec := c.getField(a, "Spec")
	bSpec := c.getField(b, "Spec")
	return reflect.DeepEqual(aSpec, bSpec)
}

func (c *Controller[T]) equalStatus(a, b T) bool {
	// Compare status fields
	aStatus := c.getField(a, "Status")
	bStatus := c.getField(b, "Status")
	return reflect.DeepEqual(aStatus, bStatus)
}

func (c *Controller[T]) getField(obj T, fieldName string) interface{} {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return nil
	}
	return field.Interface()
}

func (c *Controller[T]) copyStatus(from, to T) error {
	fromStatus := c.getField(from, "Status")
	if fromStatus == nil {
		return nil // No status to copy
	}

	toV := reflect.ValueOf(to)
	if toV.Kind() == reflect.Ptr {
		toV = toV.Elem()
	}
	if toV.Kind() != reflect.Struct {
		return fmt.Errorf("invalid object type")
	}

	statusField := toV.FieldByName("Status")
	if !statusField.IsValid() || !statusField.CanSet() {
		return fmt.Errorf("cannot set status field")
	}

	statusField.Set(reflect.ValueOf(fromStatus))
	return nil
}

// GetOwnedLister returns the lister for owned resources of the given GVK.
// Returns nil if no lister exists for that GVK.
func (c *Controller[T]) GetOwnedLister(gvk schema.GroupVersionKind) *generic.Lister[runtime.Object] {
	return c.ownedListers[gvk]
}
