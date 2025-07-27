## Experimenting with `k8s.io/client-go` and Go generics

[![Build](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml/badge.svg)](https://github.com/imjasonh/client-go2/actions/workflows/build.yaml)

This is an experimental type-parameter-aware client that wraps [`k8s.io/client-go/rest`](https://pkg.go.dev/k8s.io/client-go/rest), in [about 500 lines of mostly-vibe-coded Go](./generic/client.go).

## Features

- **Type-safe generic client** - Work with strongly-typed Kubernetes objects instead of `unstructured.Unstructured`
- **Zero code generation** - Uses Go generics instead of code generation
- **Full CRUD operations** - List, Get, Create, Update, Delete, and Patch support
- **Informer support** - Watch for changes with type-safe event handlers
- **Automatic GVR inference** - No need to manually specify GroupVersionResource for standard Kubernetes types

## Usage

See [the example](./main.go)

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
