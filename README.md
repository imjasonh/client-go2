## Experimenting with `k8s.io/client-go` and Go generics

This is an experimental type-parameter-aware client that wraps [`k8s.io/client-go/dynamic`](https://pkg.go.dev/k8s.io/client-go/dynamic) _(...for now)_.

Until Go 1.18 is released, use [`gotip`](https://pkg.go.dev/golang.org/dl/gotip).

Assuming you've got a working kubeconfig (does `kubectl get pods` work?), you can run this code:

```
$ gotip run ./
2021/09/09 12:26:06 PODS
2021/09/09 12:26:06 - coredns-558bd4d5db-hjs27
2021/09/09 12:26:06 - coredns-558bd4d5db-vhrtd
2021/09/09 12:26:06 - etcd-kind-control-plane
2021/09/09 12:26:06 - kindnet-c977m
2021/09/09 12:26:06 - kube-apiserver-kind-control-plane
2021/09/09 12:26:06 - kube-controller-manager-kind-control-plane
2021/09/09 12:26:06 - kube-proxy-fgpfd
2021/09/09 12:26:06 - kube-scheduler-kind-control-plane
2021/09/09 12:26:06 CONFIGMAPS
2021/09/09 12:26:06 - coredns
2021/09/09 12:26:06 - extension-apiserver-authentication
2021/09/09 12:26:06 - kube-proxy
2021/09/09 12:26:06 - kube-root-ca.crt
2021/09/09 12:26:06 - kubeadm-config
2021/09/09 12:26:06 - kubelet-config-1.21
```

# THIS IS AN EXPERIMENT

None of this is anywhere near set in stone.
The name `client-go2` is a placeholder, and a joke.
