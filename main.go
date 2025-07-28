package main

import (
	"context"
	"log"
	"time"

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
	pods, err := podClient.List(ctx, "kube-system", nil)
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

	// Example 1: Start an informer for ALL ConfigMaps (no selector)
	log.Println("Starting informer for ALL ConfigMaps...")
	cmc.Inform(ctx, generic.InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[ALL] ConfigMap added: %s/%s", obj.Namespace, obj.Name)
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.ConfigMap) {
			log.Printf("[ALL] ConfigMap updated: %s/%s", oldObj.Namespace, oldObj.Name)
		},
		OnDelete: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[ALL] ConfigMap deleted: %s/%s", obj.Namespace, obj.Name)
		},
		OnError: func(obj any, err error) {
			log.Printf("[ALL] Error in ConfigMap informer: %v", err)
		},
	}, nil)

	// Example 2: Start an informer with label selector
	log.Println("Starting informer for ConfigMaps with label test=example...")
	cmc.Inform(ctx, generic.InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[LABELED] ConfigMap added: %s/%s (labels: %v)", obj.Namespace, obj.Name, obj.Labels)
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.ConfigMap) {
			log.Printf("[LABELED] ConfigMap updated: %s/%s", oldObj.Namespace, oldObj.Name)
		},
		OnDelete: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[LABELED] ConfigMap deleted: %s/%s", obj.Namespace, obj.Name)
		},
		OnError: func(obj any, err error) {
			log.Printf("[LABELED] Error: %v", err)
		},
	}, &generic.InformOptions{
		ListOptions: metav1.ListOptions{
			LabelSelector: "test=example",
		},
	})

	// Example 3: Start a Pod informer with field selector for running pods only
	log.Println("Starting informer for Running Pods only...")
	podClient.Inform(ctx, generic.InformerHandler[*corev1.Pod]{
		OnAdd: func(key string, obj *corev1.Pod) {
			log.Printf("[RUNNING] Pod added: %s/%s (phase: %s)", obj.Namespace, obj.Name, obj.Status.Phase)
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.Pod) {
			if oldObj.Status.Phase != newObj.Status.Phase {
				log.Printf("[RUNNING] Pod phase changed: %s/%s (%s -> %s)",
					oldObj.Namespace, oldObj.Name, oldObj.Status.Phase, newObj.Status.Phase)
			}
		},
		OnDelete: func(key string, obj *corev1.Pod) {
			log.Printf("[RUNNING] Pod deleted: %s/%s", obj.Namespace, obj.Name)
		},
		OnError: func(obj any, err error) {
			log.Printf("[RUNNING] Error: %v", err)
		},
	}, &generic.InformOptions{
		ListOptions: metav1.ListOptions{
			FieldSelector: "status.phase=Running",
		},
	})

	// Example 4: Informer with custom resync period
	resync := 30 * time.Second
	log.Printf("Starting informer with custom resync period of %v...\n", resync)
	cmc.Inform(ctx, generic.InformerHandler[*corev1.ConfigMap]{
		OnAdd: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[RESYNC] ConfigMap added: %s/%s", obj.Namespace, obj.Name)
		},
		OnUpdate: func(key string, oldObj, newObj *corev1.ConfigMap) {
			log.Printf("[RESYNC] ConfigMap updated: %s/%s", oldObj.Namespace, oldObj.Name)
		},
		OnDelete: func(key string, obj *corev1.ConfigMap) {
			log.Printf("[RESYNC] ConfigMap deleted: %s/%s", obj.Namespace, obj.Name)
		},
		OnError: func(obj any, err error) {
			log.Printf("[RESYNC] Error: %v", err)
		},
	}, &generic.InformOptions{
		ListOptions: metav1.ListOptions{
			LabelSelector: "special=resync-test",
		},
		ResyncPeriod: &resync,
	})

	// Wait a moment for informers to sync
	time.Sleep(2 * time.Second)

	// Create ConfigMaps with different labels to demonstrate selectors
	log.Println("\nCREATING TEST RESOURCES...")

	// Create a ConfigMap without labels (will only show in ALL informer)
	cm1, err := cmc.Create(ctx, "default", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "no-labels-",
		},
		Data: map[string]string{
			"hello": "world",
		},
	}, nil)
	if err != nil {
		log.Printf("Error creating cm1: %v", err)
	} else {
		log.Printf("Created ConfigMap without labels: %s", cm1.Name)
	}

	// Create a ConfigMap with test=example label (will show in LABELED informer)
	cm2, err := cmc.Create(ctx, "default", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "labeled-",
			Labels: map[string]string{
				"test": "example",
			},
		},
		Data: map[string]string{
			"hello": "labeled",
		},
	}, nil)
	if err != nil {
		log.Printf("Error creating cm2: %v", err)
	} else {
		log.Printf("Created ConfigMap with test=example label: %s", cm2.Name)
	}

	// Create a ConfigMap with special=resync-test label (will show in RESYNC informer)
	cm3, err := cmc.Create(ctx, "default", &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "resync-test-",
			Labels: map[string]string{
				"special": "resync-test",
			},
		},
		Data: map[string]string{
			"hello": "resync",
		},
	}, nil)
	if err != nil {
		log.Printf("Error creating cm3: %v", err)
	} else {
		log.Printf("Created ConfigMap with special=resync-test label: %s", cm3.Name)
	}

	// Wait for create events
	time.Sleep(2 * time.Second)

	// Update the labeled ConfigMap
	if cm2 != nil {
		log.Printf("\nUPDATING CONFIGMAP %s", cm2.Name)
		cm2.Data["hello"] = "updated"
		_, err = cmc.Update(ctx, "default", cm2, nil)
		if err != nil {
			log.Printf("Error updating cm2: %v", err)
		}
	}

	// Wait for update events
	time.Sleep(2 * time.Second)

	// Clean up
	log.Println("\nCLEANING UP...")
	if cm1 != nil {
		if err := cmc.Delete(ctx, "default", cm1.Name, nil); err != nil {
			log.Printf("Error deleting cm1: %v", err)
		}
	}
	if cm2 != nil {
		if err := cmc.Delete(ctx, "default", cm2.Name, nil); err != nil {
			log.Printf("Error deleting cm2: %v", err)
		}
	}
	if cm3 != nil {
		if err := cmc.Delete(ctx, "default", cm3.Name, nil); err != nil {
			log.Printf("Error deleting cm3: %v", err)
		}
	}

	// Wait for delete events
	time.Sleep(2 * time.Second)

	// List ConfigMaps with label selector
	log.Println("\nLISTING CONFIGMAPS WITH LABEL test=example")
	labeledCMs, err := cmc.List(ctx, "", &metav1.ListOptions{
		LabelSelector: "test=example",
	})
	if err != nil {
		log.Printf("Error listing labeled configmaps: %v", err)
	} else {
		log.Printf("Found %d ConfigMaps with test=example label:", len(labeledCMs))
		for _, cm := range labeledCMs {
			log.Printf("- %s/%s", cm.Namespace, cm.Name)
		}
	}

	// List all ConfigMaps in kube-system
	log.Println("\nLISTING ALL CONFIGMAPS IN kube-system")
	cms, err := cmc.List(ctx, "kube-system", nil)
	if err != nil {
		log.Fatal("listing configmaps:", err)
	}
	log.Printf("Found %d ConfigMaps in kube-system:", len(cms))
	for _, cm := range cms {
		log.Println("-", cm.Name)
	}

	// Demonstrate Pod expansion methods
	log.Println("\n=== POD EXPANSION EXAMPLE ===")

	// Get a namespace-scoped PodClient with expansion methods from the existing pod client
	expandedPodClient := podClient.PodClient("kube-system")

	// List pods to find one to get logs from
	dnsPods, err := podClient.List(ctx, "kube-system", &metav1.ListOptions{
		LabelSelector: "k8s-app=kube-dns",
	})
	if err != nil {
		log.Printf("Error listing pods: %v", err)
	} else if len(dnsPods) > 0 {
		// Get logs from the first CoreDNS pod
		pod := dnsPods[0]
		log.Printf("\nGETTING LOGS FROM POD %s", pod.Name)

		// Get last 5 lines of logs
		tailLines := int64(5)
		logOpts := &corev1.PodLogOptions{
			TailLines: &tailLines,
		}

		req := expandedPodClient.GetLogs(pod.Name, logOpts)
		logs, err := req.DoRaw(ctx)
		if err != nil {
			log.Printf("Error getting logs: %v", err)
		} else {
			log.Println("Last 5 lines of logs:")
			log.Println(string(logs))
		}

		// If the pod has multiple containers, get logs from a specific container
		if len(pod.Spec.Containers) > 0 {
			containerName := pod.Spec.Containers[0].Name
			log.Printf("\nGETTING LOGS FROM CONTAINER %s", containerName)

			containerLogOpts := &corev1.PodLogOptions{
				Container: containerName,
				TailLines: &tailLines,
			}

			req := expandedPodClient.GetLogs(pod.Name, containerLogOpts)
			logs, err := req.DoRaw(ctx)
			if err != nil {
				log.Printf("Error getting container logs: %v", err)
			} else {
				log.Printf("Last 5 lines from container %s:", containerName)
				log.Println(string(logs))
			}
		}
	} else {
		log.Println("No CoreDNS pods found to demonstrate GetLogs")
	}

	// Demonstrate using the generic SubResource method
	log.Println("\n=== SUBRESOURCE EXAMPLE ===")
	if len(dnsPods) > 0 {
		pod := dnsPods[0]
		log.Printf("Getting status subresource for pod %s", pod.Name)

		req := podClient.SubResource(pod.Namespace, pod.Name, "status")
		statusBytes, err := req.DoRaw(ctx)
		if err != nil {
			log.Printf("Error getting pod status: %v", err)
		} else {
			// Just show that we got data (full status would be verbose)
			log.Printf("Got pod status (%d bytes)", len(statusBytes))
		}
	}
}
