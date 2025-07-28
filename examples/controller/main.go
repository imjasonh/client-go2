package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/imjasonh/client-go2/controller"
	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// ConfigMapReconciler validates ConfigMaps and tracks their status using annotations.
type ConfigMapReconciler struct {
	logger *slog.Logger
}

// ReconcileKind implements the reconciliation logic for ConfigMaps.
func (r *ConfigMapReconciler) ReconcileKind(ctx context.Context, cm *corev1.ConfigMap) error {
	r.logger.Info("reconciling configmap",
		"namespace", cm.Namespace,
		"name", cm.Name,
		"resourceVersion", cm.ResourceVersion)

	// Initialize annotations if needed
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}

	// Check if being deleted
	if cm.DeletionTimestamp != nil {
		if hasFinalizer(cm, "example.com/config-validator") {
			// Cleanup logic here
			r.logger.Info("cleaning up configmap", "name", cm.Name)

			// Remove our finalizer
			removeFinalizer(cm, "example.com/config-validator")
			r.logger.Info("removed finalizer", "name", cm.Name)
		}
		return nil
	}

	// Add finalizer if not present
	if !hasFinalizer(cm, "example.com/config-validator") {
		cm.Finalizers = append(cm.Finalizers, "example.com/config-validator")
		r.logger.Info("added finalizer", "name", cm.Name)
	}

	// Validate ConfigMap data
	if err := r.validateConfig(cm); err != nil {
		// Set error status in annotations
		cm.Annotations["example.com/status"] = "invalid"
		cm.Annotations["example.com/message"] = err.Error()
		cm.Annotations["example.com/validated-at"] = time.Now().Format(time.RFC3339)

		r.logger.Warn("configmap validation failed",
			"name", cm.Name,
			"error", err)

		// Requeue to check again later
		return controller.RequeueAfter(30 * time.Second)
	}

	// Set success status
	cm.Annotations["example.com/status"] = "valid"
	cm.Annotations["example.com/message"] = "configuration validated successfully"
	cm.Annotations["example.com/validated-at"] = time.Now().Format(time.RFC3339)

	r.logger.Info("configmap validated successfully", "name", cm.Name)

	// Don't requeue - we'll be notified of any changes
	return nil
}

// validateConfig checks if the ConfigMap has required keys.
func (r *ConfigMapReconciler) validateConfig(cm *corev1.ConfigMap) error {
	// Example validation: check for required keys
	requiredKeys := []string{"config.yaml", "settings.json"}

	for _, key := range requiredKeys {
		if _, exists := cm.Data[key]; !exists {
			return fmt.Errorf("missing required key: %s", key)
		}
	}

	// Check if config.yaml contains required fields
	configData, ok := cm.Data["config.yaml"]
	if ok && !strings.Contains(configData, "version:") {
		return fmt.Errorf("config.yaml must contain 'version' field")
	}

	return nil
}

// Helper functions for finalizers
func hasFinalizer(cm *corev1.ConfigMap, finalizer string) bool {
	for _, f := range cm.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func removeFinalizer(cm *corev1.ConfigMap, finalizer string) {
	var finalizers []string
	for _, f := range cm.Finalizers {
		if f != finalizer {
			finalizers = append(finalizers, f)
		}
	}
	cm.Finalizers = finalizers
}

func main() {
	var (
		kubeconfig = flag.String("kubeconfig", clientcmd.RecommendedHomeFile, "path to kubeconfig")
		namespace  = flag.String("namespace", "", "namespace to watch (empty for all)")
		workers    = flag.Int("workers", 2, "number of concurrent workers")
	)
	flag.Parse()

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Load kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("failed to load kubeconfig: %v", err)
	}

	// Create generic client for ConfigMaps
	client, err := generic.NewClient[*corev1.ConfigMap](config)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Create reconciler
	reconciler := &ConfigMapReconciler{
		logger: logger,
	}

	// Build controller
	ctrl := controller.New(client, reconciler, &controller.Options[*corev1.ConfigMap]{
		Namespace:   *namespace,
		Concurrency: *workers,
		DeepCopyFunc: func(cm *corev1.ConfigMap) *corev1.ConfigMap {
			return cm.DeepCopy()
		},
	})

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		logger.Info("received interrupt signal, shutting down...")
		cancel()
	}()

	// Create some example ConfigMaps to demonstrate the controller
	if *namespace != "" {
		createExamples(ctx, client, *namespace, logger)
	}

	// Run controller
	logger.Info("starting configmap controller",
		"namespace", *namespace,
		"workers", *workers)

	if err := ctrl.Run(ctx); err != nil {
		log.Fatalf("controller error: %v", err)
	}

	logger.Info("controller stopped")
}

// createExamples creates some ConfigMaps to demonstrate the controller.
func createExamples(ctx context.Context, client generic.Client[*corev1.ConfigMap], namespace string, logger *slog.Logger) {
	examples := []struct {
		name string
		data map[string]string
	}{
		{
			name: "valid-config",
			data: map[string]string{
				"config.yaml":   "version: 1.0\nname: example",
				"settings.json": `{"debug": false}`,
			},
		},
		{
			name: "missing-keys",
			data: map[string]string{
				"config.yaml": "name: incomplete", // missing settings.json
			},
		},
		{
			name: "invalid-config",
			data: map[string]string{
				"config.yaml":   "name: no-version", // missing version field
				"settings.json": `{"debug": true}`,
			},
		},
	}

	for _, ex := range examples {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ex.name,
				Namespace: namespace,
			},
			Data: ex.data,
		}

		// Try to create, ignore if already exists
		_, err := client.Create(ctx, namespace, cm, nil)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			logger.Error("failed to create example configmap",
				"name", ex.name,
				"error", err)
		} else if err == nil {
			logger.Info("created example configmap", "name", ex.name)
		}
	}
}
