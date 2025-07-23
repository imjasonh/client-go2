package main

import (
	"context"
	"log"

	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.Fatalf("ClientConfig: %v", err)
	}

	// List pods in kube-system using automatic GVR inference.
	podClient, err := generic.NewClient[*corev1.Pod](config)
	if err != nil {
		log.Fatal("creating pod client:", err)
	}
	pods, err := podClient.List(ctx, "kube-system")
	if err != nil {
		log.Fatal("listing pods:", err)
	}
	log.Println("LISTING PODS")
	for _, p := range pods {
		log.Println("-", p.Name)
	}

	// For ConfigMaps, we'll also use automatic GVR inference
	cmc, err := generic.NewClient[*corev1.ConfigMap](config)
	if err != nil {
		log.Fatal("creating configmap client:", err)
	}
	cmc.Start(ctx)

	// Start an informer to log all adds/updates/deletes for ConfigMaps.
	cmc.Inform(ctx, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			log.Println("--> ADD", key)
		},
		UpdateFunc: func(_, obj interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			log.Println("--> UPDATE", key)
		},
		DeleteFunc: func(obj interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			log.Println("--> DELETE", key)
		},
	})

	// Create a ConfigMap, then list ConfigMaps.
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
	log.Println("LISTING CONFIGMAPS")
	for _, cm := range cms {
		log.Println("-", cm.Name)
	}
}
