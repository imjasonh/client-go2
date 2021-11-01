package main

import (
	"context"
	"log"

	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	podGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
	cmGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
)

func main() {
	ctx := context.Background()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.Fatalf("ClientConfig: %v", err)
	}

	// List pods in kube-system.
	pods, err := generic.NewClient[*corev1.Pod](podGVR, config).List(ctx, "kube-system")
	if err != nil {
		log.Fatal("listing pods:", err)
	}
	log.Println("PODS")
	for _, p := range pods {
		log.Println("-", p.Name)
	}

	// Create a ConfigMap, then list ConfigMaps.
	cmc := generic.NewClient[*corev1.ConfigMap](cmGVR, config)
	if err := cmc.Create(ctx, "kube-system", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "foo-",
		},
		Data: map[string]string{
			"hello": "world",
		},
	}); err != nil {
		log.Fatal("creating configmap:", err)
	}
	cms, err := cmc.List(ctx, "kube-system")
	if err != nil {
		log.Fatal("listing configmaps:", err)
	}
	log.Println("CONFIGMAPS")
	for _, cm := range cms {
		log.Println("-", cm.Name)
	}
}
