# Controller Package

The `controller` package provides a generic Kubernetes controller framework inspired by Knative's controller implementation. It simplifies building Kubernetes controllers with automatic update detection, conflict resolution, and owner reference support.

## Features

- **Type-safe generic controllers** - Build controllers for any Kubernetes resource type
- **Simple reconciler interface** - Just implement one method: `ReconcileKind` or pass a function.
- **Automatic update detection** - The framework detects and persists changes to your objects
- **Built-in conflict resolution** - Automatic retry with exponential backoff
- **Owner reference support** - Automatically reconcile owners when owned resources change
- **Error handling patterns** - Control reconciliation behavior with special error types
- **Context-aware logging** - Integrated with clog for structured logging

## Quick Start

```go
import (
    "context"
    "github.com/imjasonh/client-go2/controller"
    "github.com/imjasonh/client-go2/generic"
    corev1 "k8s.io/api/core/v1"
)

// Create a reconciler function
reconciler := controller.ReconcilerFunc[*corev1.ConfigMap](
    func(ctx context.Context, cm *corev1.ConfigMap) error {
        // Your reconciliation logic here
        cm.Annotations["processed"] = "true"
        return nil
    })

// Create and run the controller
client, _ := generic.NewClient[*corev1.ConfigMap](config)
ctrl := controller.New(client, reconciler, nil)
ctrl.Run(ctx)
```

## Reconciler Interface

The core interface has just one method:

```go
type Reconciler[T runtime.Object] interface {
    ReconcileKind(ctx context.Context, obj T) error
}
```

You can implement this interface directly or use `ReconcilerFunc` for simple cases.

## Automatic Updates

The controller automatically detects and persists changes made during reconciliation:

- **Metadata changes** - Updates to annotations, labels, and finalizers
- **Status updates** - Changes to the status subresource
- **Spec changes** - Logged as warnings (specs should not be modified in reconcilers)

Example:

```go
func (r *MyReconciler) ReconcileKind(ctx context.Context, pod *corev1.Pod) error {
    // These changes will be automatically persisted
    pod.Labels["processed"] = "true"
    pod.Status.Phase = corev1.PodRunning
    
    // Add a finalizer
    pod.Finalizers = append(pod.Finalizers, "example.com/finalizer")
    
    return nil
}
```

## Error Handling

Control reconciliation behavior by returning special errors:

```go
// Requeue after a specific duration
return controller.RequeueAfter(5 * time.Minute)

// Requeue immediately with rate limiting
return controller.RequeueImmediately()

// Don't retry this error
return controller.PermanentError(fmt.Errorf("unrecoverable error"))

// Normal errors are retried with exponential backoff
return fmt.Errorf("temporary error")
```

## Controller Options

Configure the controller behavior:

```go
opts := &controller.Options[*corev1.Pod]{
    // Watch only a specific namespace
    Namespace: "my-namespace",
    
    // Number of concurrent workers
    Concurrency: 5,
    
    // Custom deep copy function (default uses JSON)
    DeepCopyFunc: func(pod *corev1.Pod) *corev1.Pod {
        return pod.DeepCopy()
    },
    
    // Watch owned resources
    OwnedTypes: []controller.OwnedType{
        {
            Client:       secretClient,
            OwnerGVK:    schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
            IsController: true,
        },
    },
}

ctrl := controller.New(client, reconciler, opts)
```

## Owner References

The controller can watch owned resources and reconcile owners when owned resources change:

```go
// In your reconciler, set owner references
func (r *MyReconciler) ReconcileKind(ctx context.Context, pod *corev1.Pod) error {
    // Create a owned secret
    secret := &corev1.Secret{...}
    
    // Set owner reference
    controller.SetOwnerReference(secret, pod, scheme.Scheme, true)
    
    // Create the secret...
    return nil
}

// Configure the controller to watch secrets
opts := &controller.Options[*corev1.Pod]{
    OwnedTypes: []controller.OwnedType{
        {
            Client:       secretClient,
            OwnerGVK:    schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
            IsController: true, // Only watch controller references
        },
    },
}
```

## Complete Example

See [examples/controller/main.go](../examples/controller/main.go) for a complete example that validates ConfigMaps.

## Best Practices

1. **Don't modify spec** - Reconcilers should only update metadata and status
2. **Use finalizers carefully** - Always check DeletionTimestamp before adding
3. **Handle conflicts** - The framework handles most conflicts, but design for idempotency
4. **Log with context** - Use clog for structured, context-aware logging
5. **Return appropriate errors** - Use RequeueAfter for known delays, PermanentError for unrecoverable failures

## Testing

The package includes comprehensive unit and e2e tests. See `controller_test.go` and `e2e_test.go` for examples of testing your controllers.
