package main

import (
	"context"
	"log"

	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{}).ClientConfig()
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

	// Start an informer to log all adds/updates/deletes for ConfigMaps.
	cmc.Inform(ctx, generic.InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, obj *corev1.ConfigMap) {
			log.Printf("ConfigMap added: %s/%s", obj.Namespace, obj.Name)
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.ConfigMap) {
			log.Printf("ConfigMap updated: %s/%s (old: %s, new: %s)", oldObj.Namespace, oldObj.Name, oldObj.Data, newObj.Data)
		},
		OnDelete: func(key string, obj *corev1.ConfigMap) {
			log.Printf("ConfigMap deleted: %s/%s", obj.Namespace, obj.Name)
		},
		OnError: func(obj any, err error) {
			log.Printf("Error in ConfigMap informer: %v (object: %v, type: %T)", err, obj, obj)
		},
	})

	// Create a ConfigMap
	cm, err := cmc.Create(ctx, "kube-system", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "foo-",
		},
		Data: map[string]string{
			"hello": "world",
		},
	})
	if err != nil {
		log.Fatal("creating configmap:", err)
	}

	// Update the ConfigMap
	cm.Data["hello"] = "universe" // Update the ConfigMap
	log.Println("UPDATING CONFIGMAP", cm.Name)
	_, err = cmc.Update(ctx, "kube-system", cm)
	if err != nil {
		log.Fatal("updating configmap:", err)
	}

	// Delete the ConfigMap
	log.Println("DELETING CONFIGMAP", cm.Name)
	if err := cmc.Delete(ctx, "kube-system", cm.Name); err != nil {
		log.Fatal("deleting configmap:", err)
	}

	// List ConfigMaps
	cms, err := cmc.List(ctx, "kube-system")
	if err != nil {
		log.Fatal("listing configmaps:", err)
	}
	log.Println("LISTING CONFIGMAPS")
	for _, cm := range cms {
		log.Println("-", cm.Name)
	}
}
