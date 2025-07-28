# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

client-go2 is an experimental Go library that provides type-safe, generic Kubernetes client operations without code generation. It wraps k8s.io/client-go/rest using Go generics to provide strongly-typed CRUD operations and informers.

## Commands

### Build and Test
```bash
# Build all packages
go build ./...

# Run unit tests with race detection
go test ./generic -v -race

# Run a specific test
go test ./generic -v -run TestNewClientGVR

# Run e2e tests (requires Kubernetes cluster)
kind create cluster  # If not already running
go test ./generic -v -tags=e2e

# Run the example/demo
go run ./main.go
```

### Code Quality
```bash
# Format code
gofmt -s -w .

# Run go vet
go vet ./...

# Ensure dependencies are tidy
go mod tidy
```

### CI/CD Verification (what runs in GitHub Actions)
```bash
go build ./...
go vet ./...
test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
go mod tidy && git diff --exit-code go.mod go.sum
go test ./generic -v -race
# Then e2e tests with Kind cluster
```

## Architecture

### Core Design
The library uses Go generics with type parameter `[T runtime.Object]` to provide compile-time type safety. There are two ways to create clients:

1. **Automatic GVR inference** (preferred): `generic.NewClient[*corev1.Pod](config)`
   - Uses Kubernetes scheme and discovery to infer GroupVersionResource from the Go type
   - Works for all types registered in the global scheme

2. **Manual GVR specification**: `generic.NewClientGVR[*corev1.Pod](gvr, config)`
   - For custom resources or when you need explicit control over the GVR

### Key Components
- `generic/client.go`: Core client implementation with CRUD operations (List, Get, Create, Update, Delete, Patch, Watch, DeleteCollection, UpdateStatus)
- `generic/expansions.go`: Resource-specific expansion methods (PodClient, ServiceClient) that implement client-go interfaces
- `generic/informer.go`: Type-safe informer implementation for watching resources
- `generic/client_test.go`: Unit tests with REST client mocking
- `generic/e2e_test.go`: Integration tests that require a real Kubernetes cluster

### Testing Approach
- Unit tests use a custom `mockTransport` that implements `http.RoundTripper`
- Mock responses are set up per HTTP method and path combination
- E2e tests create real resources in a Kind cluster and verify operations
- Prefer inline configuration pattern when creating clients in tests (see examples below)

### Expansion Methods Design
The library provides resource-specific expansion methods that maintain exact compatibility with k8s.io/client-go interfaces:

- `PodClient(namespace)` returns a namespace-scoped client implementing `typedcorev1.PodExpansion`
- `ServiceClient(namespace)` returns a namespace-scoped client implementing `typedcorev1.ServiceExpansion`
- These methods use runtime type assertions and will panic if called on the wrong type for compile-time-like safety
- Example: `client.PodClient("default").GetLogs("my-pod", opts)`

### Test Configuration Pattern
Always use the inline configuration pattern in tests:

```go
client := NewClientGVR[*corev1.Pod](
    schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
    &rest.Config{
        Host:    "http://localhost",
        APIPath: "/api",
        Transport: &mockTransport{
            responses: map[string]mockResponse{
                "GET /api/v1/namespaces/test-namespace/pods": {
                    statusCode: 200,
                    body:       string(listJSON),
                },
            },
        },
        ContentConfig: rest.ContentConfig{
            GroupVersion:         &schema.GroupVersion{Version: "v1"},
            NegotiatedSerializer: serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion(),
        },
    },
)
```

## Important Rules

1. **NEVER skip tests using `t.Skip`** - If a test is failing, fix it rather than skipping
2. When comparing structs in tests, use `github.com/google/go-cmp/cmp.Diff` for better error messages
3. Follow the existing code style - inline struct initialization, single-line error checks where appropriate
4. The library requires Go 1.22.0+ due to generic constraints
5. Always maintain exact compatibility with k8s.io/client-go interfaces - do not create custom interfaces
