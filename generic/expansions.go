package generic

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

// PodClient provides a namespace-scoped pod client that implements typedcorev1.PodExpansion.
// This matches the client-go pattern where PodInterface is already namespace-scoped.
type PodClient struct {
	client    Client[*corev1.Pod]
	namespace string
}

// Ensure we implement the interface
var _ typedcorev1.PodExpansion = PodClient{}

// GetLogs returns a request for the logs of a pod.
// This matches the signature from k8s.io/client-go/kubernetes/typed/core/v1
func (p PodClient) GetLogs(name string, opts *corev1.PodLogOptions) *rest.Request {
	req := p.client.SubResource(p.namespace, name, "log")
	if opts != nil {
		req = req.VersionedParams(opts, scheme.ParameterCodec)
	}
	return req
}

// Bind binds a pod to a node.
// This matches the signature from k8s.io/client-go/kubernetes/typed/core/v1
func (p PodClient) Bind(ctx context.Context, binding *corev1.Binding, opts metav1.CreateOptions) error {
	body, err := p.client.RESTClient().Post().
		Namespace(p.namespace).
		Resource("pods").
		Name(binding.Name).
		SubResource("binding").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(binding).
		Do(ctx).
		Raw()
	if err != nil {
		return err
	}
	// binding returns empty response
	_ = body
	return nil
}

// Evict evicts a pod using policy/v1beta1 API.
// This matches the signature from k8s.io/client-go/kubernetes/typed/core/v1
func (p PodClient) Evict(ctx context.Context, eviction *policyv1beta1.Eviction) error {
	return p.client.RESTClient().Post().
		Namespace(p.namespace).
		Resource("pods").
		Name(eviction.Name).
		SubResource("eviction").
		Body(eviction).
		Do(ctx).
		Error()
}

// EvictV1 evicts a pod using policy/v1 API.
func (p PodClient) EvictV1(ctx context.Context, eviction *policyv1.Eviction) error {
	return p.client.RESTClient().Post().
		Namespace(p.namespace).
		Resource("pods").
		Name(eviction.Name).
		SubResource("eviction").
		Body(eviction).
		Do(ctx).
		Error()
}

// EvictV1beta1 evicts a pod using policy/v1beta1 API.
func (p PodClient) EvictV1beta1(ctx context.Context, eviction *policyv1beta1.Eviction) error {
	return p.Evict(ctx, eviction)
}

// ProxyGet returns a proxy connection to the pod.
func (p PodClient) ProxyGet(scheme, name, port, path string, params map[string]string) rest.ResponseWrapper {
	request := p.client.RESTClient().Get().
		Namespace(p.namespace).
		Resource("pods").
		Name(name).
		SubResource("proxy").
		Suffix(path)
	for k, v := range params {
		request = request.Param(k, v)
	}
	return request
}

// ServiceClient provides a namespace-scoped service client that implements typedcorev1.ServiceExpansion.
// This matches the client-go pattern where ServiceInterface is already namespace-scoped.
type ServiceClient struct {
	client    Client[*corev1.Service]
	namespace string
}

// Ensure we implement the interface
var _ typedcorev1.ServiceExpansion = ServiceClient{}

// ProxyGet returns a proxy connection to the service.
func (s ServiceClient) ProxyGet(scheme, name, port, path string, params map[string]string) rest.ResponseWrapper {
	request := s.client.RESTClient().Get().
		Namespace(s.namespace).
		Resource("services").
		Name(name).
		SubResource("proxy").
		Suffix(path)
	for k, v := range params {
		request = request.Param(k, v)
	}
	return request
}
