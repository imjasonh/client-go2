package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

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

	// Set the GroupVersion in the config if not already set
	if configCopy.GroupVersion == nil {
		gv := schema.GroupVersion{Group: gvr.Group, Version: gvr.Version}
		configCopy.GroupVersion = &gv
	}

	// Set the APIPath based on whether it's a core group or not
	if configCopy.APIPath == "" {
		if gvr.Group == "" {
			configCopy.APIPath = "/api"
		} else {
			configCopy.APIPath = "/apis"
		}
	}

	// Ensure we have a negotiated serializer
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

// List retrieves a list of objects of type T from the specified namespace.
func (c Client[T]) List(ctx context.Context, namespace string) ([]T, error) {
	// Get raw response body
	body, err := c.restClient.Get().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
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
func (c Client[T]) Get(ctx context.Context, namespace, name string) (T, error) {
	// Use Raw to get the bytes and unmarshal manually
	body, err := c.restClient.Get().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(name).
		VersionedParams(&metav1.GetOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Raw()
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
func (c Client[T]) Create(ctx context.Context, namespace string, t T) (T, error) {
	body, err := c.restClient.Post().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		VersionedParams(&metav1.CreateOptions{}, scheme.ParameterCodec).
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
func (c Client[T]) Update(ctx context.Context, namespace string, t T) (T, error) {
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
		VersionedParams(&metav1.UpdateOptions{}, scheme.ParameterCodec).
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
func (c Client[T]) Delete(ctx context.Context, namespace, name string) error {
	return c.restClient.Delete().
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(name).
		VersionedParams(&metav1.DeleteOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Error()
}

// Patch applies a patch to an object of type T in the specified namespace.
func (c Client[T]) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte) error {
	_, err := c.restClient.Patch(pt).
		NamespaceIfScoped(namespace, namespace != "").
		Resource(c.gvr.Resource).
		Name(name).
		VersionedParams(&metav1.PatchOptions{}, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Raw()
	return err
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

// Inform starts an informer for the specified type T and calls the appropriate handler methods
func (c Client[T]) Inform(ctx context.Context, handler InformerHandler[T]) {
	// Create a ListWatch using rest.Client
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return c.restClient.Get().
				Resource(c.gvr.Resource).
				VersionedParams(&options, scheme.ParameterCodec).
				Do(ctx).
				Get()
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.restClient.Get().
				Resource(c.gvr.Resource).
				VersionedParams(&options, scheme.ParameterCodec).
				Watch(ctx)
		},
	}

	// Create a new informer
	var zero T
	informer := cache.NewSharedInformer(lw, zero, resyncPeriod)

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
		handler.handleErr(nil, fmt.Errorf("failed to add event handler: %w", err))
		return
	}

	go informer.Run(ctx.Done())
	if !cache.WaitForNamedCacheSync(c.gvr.String(), ctx.Done(), informer.HasSynced) {
		handler.handleErr(nil, fmt.Errorf("failed to sync informer for %s", c.gvr.String()))
		return
	}
}

func (h InformerHandler[T]) handleErr(obj any, err error) {
	if h.OnError != nil {
		h.OnError(obj, err)
	}
}
