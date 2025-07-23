package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
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
func NewClient[T runtime.Object](config *rest.Config) (client[T], error) {
	gvr, err := inferGVR[T](config)
	if err != nil {
		return client[T]{}, err
	}
	return NewClientGVR[T](gvr, config), nil
}

// NewClientGVR creates a new generic client with an explicit GroupVersionResource.
// This is useful when you need to specify a custom GVR or when the type isn't
// registered in the global scheme.
//
// Most users should prefer NewClient which automatically infers the GVR.
func NewClientGVR[T runtime.Object](gvr schema.GroupVersionResource, config *rest.Config) client[T] {
	dyn := dynamic.NewForConfigOrDie(config)
	return client[T]{
		gvr:  gvr,
		dyn:  dyn,
		dsif: dynamicinformer.NewDynamicSharedInformerFactory(dyn, resyncPeriod),
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

type client[T runtime.Object] struct {
	gvr schema.GroupVersionResource

	// TODO: don't depend on dynamic client
	dyn  dynamic.Interface
	dsif dynamicinformer.DynamicSharedInformerFactory
}

func (c client[T]) Start(ctx context.Context) {
	go c.dsif.Start(ctx.Done())
	c.dsif.WaitForCacheSync(ctx.Done())
}

func (c client[T]) List(ctx context.Context, namespace string) ([]T, error) {
	ul, err := c.dyn.Resource(c.gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []T
	for _, u := range ul.Items {
		var t T
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(u.Object); err != nil {
			return nil, err
		}
		if err := json.NewDecoder(&buf).Decode(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (c client[T]) Get(ctx context.Context, namespace, name string) (T, error) {
	var t T
	u, err := c.dyn.Resource(c.gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return t, err
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(u.Object); err != nil {
		return t, err
	}
	if err := json.NewDecoder(&buf).Decode(&t); err != nil {
		return t, err
	}
	return t, nil
}

func (c client[T]) Create(ctx context.Context, namespace string, t T) error {
	m := map[string]interface{}{}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(t); err != nil {
		return err
	}
	if err := json.NewDecoder(&buf).Decode(&m); err != nil {
		return err
	}
	u := &unstructured.Unstructured{Object: m}
	_, err := c.dyn.Resource(c.gvr).Namespace(namespace).Create(ctx, u, metav1.CreateOptions{})
	return err
}

func (c client[T]) Update(ctx context.Context, namespace string, t T) error {
	m := map[string]interface{}{}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(t); err != nil {
		return err
	}
	if err := json.NewDecoder(&buf).Decode(&m); err != nil {
		return err
	}
	u := &unstructured.Unstructured{Object: m}
	_, err := c.dyn.Resource(c.gvr).Namespace(namespace).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (c client[T]) Delete(ctx context.Context, namespace, name string) error {
	return c.dyn.Resource(c.gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c client[T]) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte) error {
	_, err := c.dyn.Resource(c.gvr).Namespace(namespace).Patch(ctx, name, pt, data, metav1.PatchOptions{})
	return err
}

func (c client[T]) Inform(ctx context.Context, handler cache.ResourceEventHandler) {
	inf := c.dsif.ForResource(c.gvr).Informer()
	inf.AddEventHandler(handler)
	go inf.Run(ctx.Done())
	if !cache.WaitForNamedCacheSync(c.gvr.String(), ctx.Done(), inf.HasSynced) {
		log.Println("Failed to wait for caches to sync:", c.gvr.String())
		return
	}
}
