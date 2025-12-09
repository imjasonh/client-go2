package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"unicode"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
)

const resyncPeriod = time.Hour

// NewClient creates a new generic client by automatically inferring
// the GroupVersionResource from the type parameter T.
// This uses the global Kubernetes scheme to look up the GVK for the type,
// then uses discovery to map that to a GVR.
//
// Note: T must be a pointer type (e.g., *corev1.Pod) as required by runtime.Object.
// Non-pointer types will fail at compile time.
func NewClient[T runtime.Object](config *rest.Config) (Client[T], error) {
	gvr, err := inferGVR[T](config)
	if err != nil {
		return Client[T]{}, err
	}
	return NewClientGVR[T](gvr, config), nil
}

// NewClientGVR creates a new generic client with an explicit GroupVersionResource.
// This is useful when you need to specify a custom GVR or when the type isn't
// registered in the global scheme.
//
// Most users should prefer NewClient which automatically infers the GVR.
func NewClientGVR[T runtime.Object](gvr schema.GroupVersionResource, config *rest.Config) Client[T] {
	// Create a copy of the config to avoid modifying the original
	configCopy := rest.CopyConfig(config)

	// For CRDs (non-empty group), include full path and use core v1 for parameter encoding
	// For built-in types (empty group), use traditional GroupVersion approach
	if gvr.Group != "" {
		// CRD: Full path in APIPath, core v1 for GroupVersion (fixes parameter encoding)
		configCopy.APIPath = "/apis/" + gvr.Group + "/" + gvr.Version
		configCopy.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	} else {
		// Built-in type: Use traditional approach
		gv := schema.GroupVersion{Group: gvr.Group, Version: gvr.Version}
		if configCopy.GroupVersion == nil {
			configCopy.GroupVersion = &gv
		}
		if configCopy.APIPath == "" {
			configCopy.APIPath = "/api"
		}
	}

	// Use the standard Kubernetes codecs for serialization
	if configCopy.NegotiatedSerializer == nil {
		configCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}

	restClient, err := rest.RESTClientFor(configCopy)
	if err != nil {
		panic(fmt.Errorf("failed to create REST client: %w", err))
	}
	return Client[T]{
		gvr:        gvr,
		restClient: restClient,
	}
}

// inferGVR attempts to determine the GroupVersionResource for a given type T
// by using the Kubernetes scheme and discovery client.
func inferGVR[T runtime.Object](config *rest.Config) (schema.GroupVersionResource, error) {
	// Create a zero-value instance of T to inspect
	var zero T
	typ := reflect.TypeOf(zero)

	// Require pointer types - Kubernetes objects should always be pointers
	if typ.Kind() != reflect.Ptr {
		return schema.GroupVersionResource{}, fmt.Errorf("type %T must be a pointer type (e.g., *corev1.Pod, not corev1.Pod)", zero)
	}

	typ = typ.Elem()
	// Create a new instance of the underlying type
	instance := reflect.New(typ).Interface()

	// Try to convert to runtime.Object
	obj, ok := instance.(runtime.Object)
	if !ok {
		return schema.GroupVersionResource{}, fmt.Errorf("type %T does not implement runtime.Object", instance)
	}

	// Get the GVKs for this object from the scheme
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get GVK for type %T: %w", zero, err)
	}

	if len(gvks) == 0 {
		return schema.GroupVersionResource{}, fmt.Errorf("no GVK registered for type %T", zero)
	}

	// If multiple match, return an error.
	if len(gvks) > 1 {
		return schema.GroupVersionResource{}, fmt.Errorf("multiple GVKs registered for type %T: %v", zero, gvks)
	}
	gvk := gvks[0]

	// Create a discovery client to get the REST mapping
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Get the API group resources
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get API group resources: %w", err)
	}

	// Create a REST mapper
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	// Get the resource mapping for the GVK
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get REST mapping for %v: %w", gvk, err)
	}

	return mapping.Resource, nil
}

// Client is a generic Kubernetes client for a specific type T.
type Client[T runtime.Object] struct {
	gvr        schema.GroupVersionResource
	restClient *rest.RESTClient
}

// isCRD returns true if this client was configured for a CRD (non-empty group)
func (c Client[T]) isCRD() bool {
	return c.gvr.Group != ""
}

// resourcePath returns the base path for this resource, accounting for namespace
func (c Client[T]) resourcePath(namespace string) string {
	if !c.isCRD() {
		return "" // Not used for built-in types
	}

	// CRD: Build full path from GVR
	path := "/apis/" + c.gvr.Group + "/" + c.gvr.Version
	if namespace != "" {
		path = path + "/namespaces/" + namespace
	}
	return path + "/" + c.gvr.Resource
}

// GVK returns the GroupVersionKind for this client.
// Note: This is an approximation since we only have GVR. The Kind is derived
// from the resource name by capitalizing and singularizing it.
func (c Client[T]) GVK() schema.GroupVersionKind {
	// Simple singularization - just remove trailing 's'
	// This won't work for all cases but covers most common ones
	kind := c.gvr.Resource
	if len(kind) > 1 && kind[len(kind)-1] == 's' {
		kind = kind[:len(kind)-1]
	}
	// Capitalize first letter
	if len(kind) > 0 {
		kind = string(unicode.ToUpper(rune(kind[0]))) + kind[1:]
	}

	return schema.GroupVersionKind{
		Group:   c.gvr.Group,
		Version: c.gvr.Version,
		Kind:    kind,
	}
}

// PodClient returns a PodClient with expansion methods.
// This will panic if T is not *corev1.Pod.
func (c Client[T]) PodClient(namespace string) PodClient {
	// Type assert to ensure T is *corev1.Pod
	var zero T
	if _, ok := any(zero).(*corev1.Pod); !ok {
		panic(fmt.Sprintf("PodClient() can only be called on Client[*corev1.Pod], not Client[%T]", zero))
	}

	// This is safe because we know T is *corev1.Pod
	podClient := any(c).(Client[*corev1.Pod])
	return PodClient{client: podClient, namespace: namespace}
}

// ServiceClient returns a ServiceClient with expansion methods.
// This will panic if T is not *corev1.Service.
func (c Client[T]) ServiceClient(namespace string) ServiceClient {
	// Type assert to ensure T is *corev1.Service
	var zero T
	if _, ok := any(zero).(*corev1.Service); !ok {
		panic(fmt.Sprintf("ServiceClient() can only be called on Client[*corev1.Service], not Client[%T]", zero))
	}

	// This is safe because we know T is *corev1.Service
	serviceClient := any(c).(Client[*corev1.Service])
	return ServiceClient{client: serviceClient, namespace: namespace}
}

// List retrieves a list of objects of type T from the specified namespace.
func (c Client[T]) List(ctx context.Context, namespace string, opts *metav1.ListOptions) ([]T, error) {
	if opts == nil {
		opts = &metav1.ListOptions{}
	}
	// Get raw response body
	body, err := c.restClient.Get().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(opts, scheme.ParameterCodec).
		Do(ctx).
		Raw()
	if err != nil {
		return nil, err
	}

	// Parse as a generic list to extract items
	var listData struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &listData); err != nil {
		return nil, err
	}

	var out []T
	for _, item := range listData.Items {
		var t T
		if err := json.Unmarshal(item, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Get retrieves a single object of type T by name from the specified namespace.
func (c Client[T]) Get(ctx context.Context, namespace, name string, opts *metav1.GetOptions) (T, error) {
	if opts == nil {
		opts = &metav1.GetOptions{}
	}

	var body []byte
	var err error
	if c.isCRD() {
		// CRD: Use AbsPath
		path := c.resourcePath(namespace) + "/" + name
		body, err = c.restClient.Get().
			AbsPath(path).
			VersionedParams(opts, scheme.ParameterCodec).
			Do(ctx).
			Raw()
	} else {
		// Built-in: Use Resource()
		body, err = c.restClient.Get().
			NamespaceIfScoped(namespace, namespace != "").
			Resource(c.gvr.Resource).
			Name(name).
			VersionedParams(opts, scheme.ParameterCodec).
			Do(ctx).
			Raw()
	}
	if err != nil {
		var zero T
		return zero, err
	}

	var t T
	if err := json.Unmarshal(body, &t); err != nil {
		var zero T
		return zero, err
	}
	return t, nil
}

// Create creates a new object of type T in the specified namespace.
func (c Client[T]) Create(ctx context.Context, namespace string, t T, opts *metav1.CreateOptions) (T, error) {
	if opts == nil {
		opts = &metav1.CreateOptions{}
	}
	body, err := c.restClient.Post().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(t).
		Do(ctx).
		Raw()
	if err != nil {
		var zero T
		return zero, err
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// Update updates an existing object of type T in the specified namespace.
func (c Client[T]) Update(ctx context.Context, namespace string, t T, opts *metav1.UpdateOptions) (T, error) {
	if opts == nil {
		opts = &metav1.UpdateOptions{}
	}
	// Extract the name from the object metadata
	data, err := json.Marshal(t)
	if err != nil {
		var zero T
		return zero, err
	}
	var meta metav1.ObjectMeta
	var objMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &objMap); err != nil {
		var zero T
		return zero, err
	}
	if metaData, ok := objMap["metadata"]; ok {
		if err := json.Unmarshal(metaData, &meta); err != nil {
			var zero T
			return zero, err
		}
	}

	body, err := c.restClient.Put().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(meta.Name).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(t).
		Do(ctx).
		Raw()
	if err != nil {
		var zero T
		return zero, err
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// Delete deletes an object of type T by name from the specified namespace.
func (c Client[T]) Delete(ctx context.Context, namespace, name string, opts *metav1.DeleteOptions) error {
	if opts == nil {
		opts = &metav1.DeleteOptions{}
	}
	return c.restClient.Delete().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(name).
		VersionedParams(opts, scheme.ParameterCodec).
		Do(ctx).
		Error()
}

// Patch applies a patch to an object of type T in the specified namespace.
func (c Client[T]) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts *metav1.PatchOptions) error {
	if opts == nil {
		opts = &metav1.PatchOptions{}
	}
	_, err := c.restClient.Patch(pt).
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(name).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Raw()
	return err
}

// Watch returns a watch interface for watching changes to resources of type T.
func (c Client[T]) Watch(ctx context.Context, namespace string, opts *metav1.ListOptions) (watch.Interface, error) {
	if opts == nil {
		opts = &metav1.ListOptions{}
	}
	opts.Watch = true
	return c.restClient.Get().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(opts, scheme.ParameterCodec).
		Watch(ctx)
}

// DeleteCollection deletes a collection of objects of type T.
func (c Client[T]) DeleteCollection(ctx context.Context, namespace string, opts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	if opts == nil {
		opts = &metav1.DeleteOptions{}
	}
	if listOpts == nil {
		listOpts = &metav1.ListOptions{}
	}
	return c.restClient.Delete().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(opts, scheme.ParameterCodec).
		VersionedParams(listOpts, scheme.ParameterCodec).
		Do(ctx).
		Error()
}

// UpdateStatus updates the status subresource of an object of type T.
func (c Client[T]) UpdateStatus(ctx context.Context, namespace string, t T, opts *metav1.UpdateOptions) (T, error) {
	if opts == nil {
		opts = &metav1.UpdateOptions{}
	}
	// Extract the name from the object metadata
	data, err := json.Marshal(t)
	if err != nil {
		var zero T
		return zero, err
	}
	var meta metav1.ObjectMeta
	var objMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &objMap); err != nil {
		var zero T
		return zero, err
	}
	if metaData, ok := objMap["metadata"]; ok {
		if err := json.Unmarshal(metaData, &meta); err != nil {
			var zero T
			return zero, err
		}
	}

	var body []byte
	if c.isCRD() {
		// CRD: Use AbsPath for the full resource path
		path := c.resourcePath(namespace) + "/" + meta.Name + "/status"
		body, err = c.restClient.Put().
			AbsPath(path).
			VersionedParams(opts, scheme.ParameterCodec).
			Body(t).
			Do(ctx).
			Raw()
	} else {
		// Built-in: Use Resource()
		body, err = c.restClient.Put().
			NamespaceIfScoped(namespace, namespace != "").
			Resource(c.gvr.Resource).
			Name(meta.Name).
			SubResource("status").
			VersionedParams(opts, scheme.ParameterCodec).
			Body(t).
			Do(ctx).
			Raw()
	}
	if err != nil {
		var zero T
		return zero, err
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// InformerHandler defines the interface for handling events from an informer.
type InformerHandler[T runtime.Object] struct {
	// OnAdd is called when a new object is added to the informer.
	OnAdd func(key string, obj T)
	// OnUpdate is called when an existing object is updated in the informer.
	OnUpdate func(key string, oldObj, newObj T)
	// OnDelete is called when an object is deleted from the informer.
	OnDelete func(key string, obj T)
	// OnError is called when an error occurs in the informer.
	OnError func(obj any, err error)
}

// InformOptions contains options for configuring an informer
type InformOptions struct {
	// ListOptions allows setting label selectors, field selectors, etc.
	ListOptions metav1.ListOptions
	// ResyncPeriod overrides the default resync period if set
	ResyncPeriod *time.Duration
}

// Inform starts an informer for the specified type T and calls the appropriate handler methods
//
// It returns a Lister[T] that can be used to list objects in the cache.
func (c Client[T]) Inform(ctx context.Context, handler InformerHandler[T], opts *InformOptions) (*Lister[T], error) {
	// Create a ListWatch using rest.Client with label selector support
	lw := &cache.ListWatch{
		ListFunc: func(listOpts metav1.ListOptions) (runtime.Object, error) {
			// Merge provided options with runtime options
			if opts != nil {
				if opts.ListOptions.LabelSelector != "" {
					listOpts.LabelSelector = opts.ListOptions.LabelSelector
				}
				if opts.ListOptions.FieldSelector != "" {
					listOpts.FieldSelector = opts.ListOptions.FieldSelector
				}
			}
			if c.isCRD() {
				return c.restClient.Get().
					AbsPath(c.resourcePath("")).
					VersionedParams(&listOpts, scheme.ParameterCodec).
					Do(ctx).
					Get()
			}
			return c.restClient.Get().
				Resource(c.gvr.Resource).
				VersionedParams(&listOpts, scheme.ParameterCodec).
				Do(ctx).
				Get()
		},
		WatchFunc: func(watchOpts metav1.ListOptions) (watch.Interface, error) {
			// Merge provided options with runtime options
			if opts != nil {
				if opts.ListOptions.LabelSelector != "" {
					watchOpts.LabelSelector = opts.ListOptions.LabelSelector
				}
				if opts.ListOptions.FieldSelector != "" {
					watchOpts.FieldSelector = opts.ListOptions.FieldSelector
				}
			}
			if c.isCRD() {
				return c.restClient.Get().
					AbsPath(c.resourcePath("")).
					VersionedParams(&watchOpts, scheme.ParameterCodec).
					Watch(ctx)
			}
			return c.restClient.Get().
				Resource(c.gvr.Resource).
				VersionedParams(&watchOpts, scheme.ParameterCodec).
				Watch(ctx)
		},
	}

	// Set default resync period
	resync := resyncPeriod
	if opts != nil && opts.ResyncPeriod != nil {
		resync = *opts.ResyncPeriod
	}

	// Create a new informer
	var zero T
	informer := cache.NewSharedIndexInformer(lw, zero, resync, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if handler.OnAdd == nil {
				return
			}
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				handler.handleErr(obj, err)
				return
			}
			t, ok := obj.(T)
			if !ok {
				handler.handleErr(obj, fmt.Errorf("expected type %T, got %T", zero, obj))
				return
			}
			handler.OnAdd(key, t)
		},
		UpdateFunc: func(oldObj, newObj any) {
			if handler.OnUpdate == nil {
				return
			}
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err != nil {
				handler.handleErr(newObj, err)
				return
			}
			oldT, ok := oldObj.(T)
			if !ok {
				handler.handleErr(oldObj, fmt.Errorf("failed to cast old object to expected type: %v", oldObj))
				return
			}
			newT, ok := newObj.(T)
			if !ok {
				handler.handleErr(newObj, fmt.Errorf("failed to cast new object to expected type: %v", newObj))
				return
			}
			handler.OnUpdate(key, oldT, newT)
		},
		DeleteFunc: func(obj any) {
			if handler.OnDelete == nil {
				return
			}
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				handler.handleErr(obj, fmt.Errorf("failed to get key for deleted object: %v", obj))
				return
			}
			objT, ok := obj.(T)
			if !ok {
				handler.handleErr(obj, fmt.Errorf("failed to cast deleted object to expected type: %v", obj))
				return
			}
			handler.OnDelete(key, objT)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add event handler: %w", err)
	}

	go informer.Run(ctx.Done())
	if !cache.WaitForNamedCacheSync(c.gvr.String(), ctx.Done(), informer.HasSynced) {
		return nil, fmt.Errorf("failed to sync informer for %s", c.gvr.String())
	}
	return NewLister[T](informer, c.gvr.GroupResource()), nil
}

// SubResource returns a request for a subresource of the given resource.
// This can be used to access subresources like logs, exec, attach, etc.
// For example, to get pod logs:
//
//	req := client.SubResource("default", "my-pod", "log")
//	req.VersionedParams(&v1.PodLogOptions{...}, scheme.ParameterCodec)
func (c Client[T]) SubResource(namespace, name, subresource string) *rest.Request {
	return c.restClient.Get().
		NamespaceIfScoped(namespace, namespace != "").
		Name(name).
		Resource(c.gvr.Resource).
		SubResource(subresource)
}

// RESTClient returns the underlying rest.RESTClient.
// This is useful for advanced use cases where direct access to the REST client is needed.
func (c Client[T]) RESTClient() *rest.RESTClient {
	return c.restClient
}

func (h InformerHandler[T]) handleErr(obj any, err error) {
	if h.OnError != nil {
		h.OnError(obj, err)
	}
}
