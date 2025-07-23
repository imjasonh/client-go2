## Experimenting with `k8s.io/client-go` and Go generics

[![Build](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml/badge.svg)](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml)

This is an experimental type-parameter-aware client that wraps [`k8s.io/client-go/rest`](https://pkg.go.dev/k8s.io/client-go/rest).

## Features

- **Type-safe generic client** - Work with strongly-typed Kubernetes objects instead of `unstructured.Unstructured`
- **Zero code generation** - Uses Go generics instead of code generation
- **Full CRUD operations** - List, Get, Create, Update, Delete, and Patch support
- **Informer support** - Watch for changes with type-safe event handlers
- **Automatic GVR inference** - No need to manually specify GroupVersionResource for standard Kubernetes types

## Usage

```go
// Create a client that automatically infers the GVR from the type
podClient, err := generic.NewClient[*corev1.Pod](config)
if err != nil {
    log.Fatal(err)
}

// List pods in a namespace
pods, err := podClient.List(ctx, "kube-system")
if err != nil {
    log.Fatal(err)
}

// Create a ConfigMap
cmClient, err := generic.NewClient[*corev1.ConfigMap](config)
if err != nil {
    log.Fatal(err)
}

err = cmClient.Create(ctx, "default", &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name: "my-config",
    },
    Data: map[string]string{
        "key": "value",
    },
})

// If you need to specify a custom GVR, use NewClientGVR
customClient := generic.NewClientGVR[*corev1.Pod](customGVR, config)
```

## Running the Example

```
$ go run ./
2025/07/23 11:23:10 LISTING PODS
2025/07/23 11:23:10 - coredns-674b8bbfcf-ddcww
2025/07/23 11:23:10 - coredns-674b8bbfcf-dqqx5
2025/07/23 11:23:10 - etcd-kind-control-plane
2025/07/23 11:23:10 - kindnet-tkf6l
2025/07/23 11:23:10 - kube-apiserver-kind-control-plane
2025/07/23 11:23:10 - kube-controller-manager-kind-control-plane
2025/07/23 11:23:10 - kube-proxy-76tcd
2025/07/23 11:23:10 - kube-scheduler-kind-control-plane
...
```

## Testing

Run unit tests:
```bash
go test ./generic
```

Run e2e tests (requires a Kubernetes cluster):
```bash
go test ./generic -tags=e2e
```

# THIS IS AN EXPERIMENT

None of this is anywhere near set in stone.
The name `client-go2` is a placeholder, and a joke.
