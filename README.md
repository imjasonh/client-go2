## Experimenting with `k8s.io/client-go` and Go generics

[![Build](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml/badge.svg)](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml)

This is an experimental type-parameter-aware client that wraps [`k8s.io/client-go/rest`](https://pkg.go.dev/k8s.io/client-go/rest), in [about 570 lines of mostly-vibe-coded Go](./generic/client.go).

## Features

- **Type-safe generic client** - Work with strongly-typed Kubernetes objects instead of `unstructured.Unstructured`
- **Zero code generation** - Uses Go generics instead of code generation
- **Full CRUD operations** - List, Get, Create, Update, Delete, Patch, Watch, DeleteCollection, and UpdateStatus support
- **Informer support** - Watch for changes with type-safe event handlers
- **Automatic GVR inference** - No need to manually specify GroupVersionResource for standard Kubernetes types
- **Expansion methods** - Resource-specific operations like Pod.GetLogs() and Service.ProxyGet()
- **Label/Field selectors** - Filter resources using Kubernetes selectors
- **SubResource access** - Generic method to access any subresource

## Usage

See [the example](./main.go) for comprehensive usage examples.

### Quick Examples

#### Basic CRUD Operations
```go
// Create a client with automatic GVR inference
client, err := generic.NewClient[*corev1.Pod](config)

// List pods with label selector
pods, err := client.List(ctx, "default", &metav1.ListOptions{
    LabelSelector: "app=nginx",
})

// Get a specific pod
pod, err := client.Get(ctx, "default", "my-pod", nil)
```

#### Expansion Methods for Pods (client-go compatible)
```go
// Start with a generic client for pods
client, err := generic.NewClient[*corev1.Pod](config)

// Get a namespace-scoped PodClient that implements typedcorev1.PodExpansion
podClient := client.PodClient("default")  // Will panic if T is not *corev1.Pod

// Get pod logs (matches client-go API)
req := podClient.GetLogs("my-pod", &corev1.PodLogOptions{
    Container: "nginx",
    TailLines: &tailLines,
})
logs, err := req.DoRaw(ctx)

// Bind pod to node
err = podClient.Bind(ctx, binding, metav1.CreateOptions{})

// Evict pod
err = podClient.Evict(ctx, eviction)

// For cluster-scoped operations, use the generic client directly
pods, err := client.List(ctx, "default", nil)
```

#### Expansion Methods for Services (client-go compatible)
```go
// Start with a generic client for services
client, err := generic.NewClient[*corev1.Service](config)

// Get a namespace-scoped ServiceClient that implements typedcorev1.ServiceExpansion
serviceClient := client.ServiceClient("default")  // Will panic if T is not *corev1.Service

// Use proxy to access service endpoints
req := serviceClient.ProxyGet("http", "my-service", "80", "api/health", nil)
resp, err := req.DoRaw(ctx)
```

#### Generic SubResource Access
```go
// Access any subresource using the generic method
req := client.SubResource("default", "my-pod", "status")
status, err := req.DoRaw(ctx)
```

#### Watch Resources
```go
// Watch for pod changes with label selector
watcher, err := client.Watch(ctx, "default", &metav1.ListOptions{
    LabelSelector: "app=nginx",
})

defer watcher.Stop()

for event := range watcher.ResultChan() {
    pod := event.Object.(*corev1.Pod)
    fmt.Printf("Event: %s Pod: %s\n", event.Type, pod.Name)
}
```

#### Delete Collection
```go
// Delete all pods with specific label
err = client.DeleteCollection(ctx, "default", nil, &metav1.ListOptions{
    LabelSelector: "app=test",
})

// Delete all pods in namespace
err = client.DeleteCollection(ctx, "default", nil, nil)
```

#### Update Status
```go
// Update only the status subresource
pod.Status.Phase = corev1.PodRunning
pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
    Type:   corev1.PodReady,
    Status: corev1.ConditionTrue,
})

updated, err := client.UpdateStatus(ctx, "default", pod, nil)
```

## Testing

Unit tests against a mock REST client:

```
make test
```

End-to-end tests against a real K8s cluster:

```
make e2e
```

# ⚠️ THIS IS AN EXPERIMENT

This is just for demo purposes.

The name `client-go2` is a placeholder, and a joke.
