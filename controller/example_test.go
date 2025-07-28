package controller_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/imjasonh/client-go2/controller"
	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

// Example demonstrates basic controller usage with a function reconciler.
func Example_basic() {
	// Create a client (normally from kubeconfig)
	config := &rest.Config{Host: "https://kubernetes.default.svc"}
	client, _ := generic.NewClient[*corev1.Pod](config)

	// Run the controller
	_ = controller.New(client, controller.ReconcilerFunc[*corev1.Pod](func(ctx context.Context, pod *corev1.Pod) error {
		log.Printf("Reconciling pod %s/%s", pod.Namespace, pod.Name)

		// Update pod status
		if pod.Status.Phase == "" {
			pod.Status.Phase = corev1.PodPending
		}

		return nil
	}), nil).Run(context.Background())
}

// PodReconciler is an example reconciler implementation.
type PodReconciler struct {
	// Add dependencies here
}

// ReconcileKind implements the Reconciler interface.
func (r *PodReconciler) ReconcileKind(ctx context.Context, pod *corev1.Pod) error {
	// Complex reconciliation logic
	if pod.DeletionTimestamp != nil {
		// Handle deletion
		return nil
	}

	// Update status based on conditions
	if pod.Status.Phase == corev1.PodRunning {
		// Pod is running, check if service is needed
		return nil
	}

	// Requeue to check again later
	return controller.RequeueAfter(30 * time.Second)
}

// Example_withInterface demonstrates using a full reconciler implementation.
func Example_withInterface() {

	// Create and run controller
	config := &rest.Config{Host: "https://kubernetes.default.svc"}
	client, _ := generic.NewClient[*corev1.Pod](config)

	reconciler := &PodReconciler{}
	_ = controller.New(client, reconciler, &controller.Options[*corev1.Pod]{
		Namespace:   "default",
		Concurrency: 5,
	}).Run(context.Background())
}

// Example_errorHandling demonstrates the different error handling options.
func Example_errorHandling() {
	// Create a client (normally from kubeconfig)
	config := &rest.Config{Host: "https://kubernetes.default.svc"}
	client, _ := generic.NewClient[*corev1.ConfigMap](config)

	// Run the controller
	_ = controller.New(client, controller.ReconcilerFunc[*corev1.ConfigMap](func(ctx context.Context, cm *corev1.ConfigMap) error {
		// Validate the ConfigMap
		if cm.Data["config"] == "" {
			// Requeue after 1 minute to check again
			return controller.RequeueAfter(time.Minute)
		}

		// Check for temporary issue
		if cm.Annotations["ready"] != "true" {
			// Requeue immediately
			return controller.RequeueImmediately()
		}

		// Check for permanent failure
		if cm.Data["invalid"] == "true" {
			// Don't retry this error
			return controller.PermanentError(
				fmt.Errorf("configmap is permanently invalid"))
		}

		// Success - no requeue needed
		return nil
	}), nil).Run(context.Background())
}
