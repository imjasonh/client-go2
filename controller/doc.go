// Package controller provides a generic Kubernetes controller implementation
// that works with the generic client from github.com/imjasonh/client-go2/generic.
//
// The controller package makes it easy to build Kubernetes controllers with
// minimal boilerplate while providing production-ready features like automatic
// status updates, conflict resolution, and error handling.
//
// # Basic Usage
//
// To create a controller, you need a generic client and a reconciler:
//
//	client, _ := generic.NewClient[*corev1.Pod](config)
//
//	ctrl, _ := controller.NewBuilder(client).
//	    ForFunc(func(ctx context.Context, pod *corev1.Pod) error {
//	        // Reconciliation logic
//	        if pod.Status.Phase == "" {
//	            pod.Status.Phase = corev1.PodPending
//	        }
//	        return nil
//	    }).
//	    Build()
//
//	ctrl.Run(ctx)
//
// # Reconciler Interface
//
// For more complex reconcilers, implement the Reconciler interface:
//
//	type MyReconciler struct {
//	    // dependencies
//	}
//
//	func (r *MyReconciler) ReconcileKind(ctx context.Context, pod *corev1.Pod) error {
//	    // Complex reconciliation logic
//	    return nil
//	}
//
// # Automatic Updates
//
// The controller automatically persists changes made to the object during
// reconciliation:
//   - Status changes are persisted via UpdateStatus
//   - Finalizer changes are persisted via Update
//   - Spec and other metadata changes are ignored (and logged)
//
// This behavior matches Knative's reconciler pattern.
//
// # Error Handling
//
// The controller supports special error types for controlling requeue behavior:
//
//	return controller.RequeueAfter(5 * time.Second)  // Requeue after delay
//	return controller.RequeueImmediately()            // Requeue now
//	return controller.PermanentError(err)             // Don't retry
//
// Regular errors result in exponential backoff retries.
//
// # Conflict Resolution
//
// When updating objects, the controller automatically handles conflicts by
// retrying with the latest version of the object. This ensures updates
// succeed even under contention.
package controller
