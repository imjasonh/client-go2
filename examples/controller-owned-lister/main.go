package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chainguard-dev/clog"
	"github.com/imjasonh/client-go2/controller"
	"github.com/imjasonh/client-go2/generic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/clientcmd"
)

// ConfigMapReconciler reconciles ConfigMaps and can access owned Secrets via lister
type ConfigMapReconciler struct {
	secretLister *generic.Lister[*corev1.Secret]
}

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, cm *corev1.ConfigMap) error {
	log := clog.FromContext(ctx)

	// Check if secretLister is available
	if r.secretLister == nil {
		log.Info("Secret lister not available yet")
		return nil
	}

	// List secrets in the same namespace
	secrets, err := r.secretLister.ByNamespace(cm.Namespace).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	log.Info("Reconciling ConfigMap",
		"name", cm.Name,
		"namespace", cm.Namespace,
		"allSecrets", len(secrets))

	// Look for secrets owned by this ConfigMap
	var ownedSecrets []*corev1.Secret
	for _, secret := range secrets {
		for _, owner := range secret.OwnerReferences {
			if owner.UID == cm.UID {
				ownedSecrets = append(ownedSecrets, secret)
				break
			}
		}
	}

	if len(ownedSecrets) == 0 {
		log.Info("No owned secrets found for ConfigMap", "name", cm.Name)
		return nil
	}

	log.Info("Found owned secrets", "count", len(ownedSecrets))

	// Update ConfigMap annotation with secret count
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}
	cm.Annotations["owned-secrets-count"] = fmt.Sprintf("%d", len(ownedSecrets))

	return nil
}

func main() {
	ctx := context.Background()

	// Load config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create clients
	cmClient, err := generic.NewClient[*corev1.ConfigMap](config)
	if err != nil {
		log.Fatalf("Failed to create ConfigMap client: %v", err)
	}

	// Create the reconciler
	reconciler := &ConfigMapReconciler{}

	// Create controller
	ctrl := controller.New(cmClient, reconciler, &controller.Options[*corev1.ConfigMap]{
		Namespace: "default",
	})

	// Start the controller in the background so we can set up owned resources
	go func() {
		if err := ctrl.Run(ctx); err != nil {
			log.Fatalf("Controller error: %v", err)
		}
	}()

	// Create a Secret client to get a typed lister
	secretClient, err := generic.NewClient[*corev1.Secret](config)
	if err != nil {
		log.Fatalf("Failed to create Secret client: %v", err)
	}

	// Start a Secret informer to get the lister
	secretLister, err := secretClient.Inform(ctx, generic.InformerHandler[*corev1.Secret]{
		// Empty handler - we just want the lister for cache-backed operations
	}, nil)
	if err != nil {
		log.Fatalf("Failed to start Secret informer: %v", err)
	}

	reconciler.secretLister = secretLister

	log.Println("Controller is running with secret lister available")

	// Keep running
	<-ctx.Done()
}
