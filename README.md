## Experimenting with `k8s.io/client-go` and Go generics

This is an experimental type-parameter-aware client that wraps [`k8s.io/client-go/dynamic`](https://pkg.go.dev/k8s.io/client-go/dynamic) _(...for now)_.

Until Go 1.18 is released, use [`gotip`](https://pkg.go.dev/golang.org/dl/gotip), or the recently released [Go 1.18 beta](https://go.dev/blog/go1.18beta1).

Assuming you've got a working kubeconfig (does `kubectl get pods` work?), you can run this code:

```
$ go1.18beta1 run ./
2021/12/15 12:08:11 LISTING PODS
2021/12/15 12:08:11 - coredns-558bd4d5db-hjs27
2021/12/15 12:08:11 - coredns-558bd4d5db-vhrtd
2021/12/15 12:08:11 - etcd-kind-control-plane
2021/12/15 12:08:11 - kindnet-c977m
2021/12/15 12:08:11 - kube-apiserver-kind-control-plane
2021/12/15 12:08:11 - kube-controller-manager-kind-control-plane
2021/12/15 12:08:11 - kube-proxy-fgpfd
2021/12/15 12:08:11 - kube-scheduler-kind-control-plane
I1215 12:08:11.086322   32526 shared_informer.go:240] Waiting for caches to sync for /v1, Resource=configmaps
2021/12/15 12:08:11 --> ADD kube-public/cluster-info
2021/12/15 12:08:11 --> ADD kube-system/extension-apiserver-authentication
2021/12/15 12:08:11 --> ADD tekton-pipelines/config-logging
...
2021/12/15 12:08:11 LISTING CONFIGMAPS
2021/12/15 12:08:11 - coredns
2021/12/15 12:08:11 - extension-apiserver-authentication
2021/12/15 12:08:11 - kube-proxy
2021/12/15 12:08:11 - kube-root-ca.crt
2021/12/15 12:08:11 - kubeadm-config
2021/12/15 12:08:11 - kubelet-config-1.21
```

# THIS IS AN EXPERIMENT

None of this is anywhere near set in stone.
The name `client-go2` is a placeholder, and a joke.
