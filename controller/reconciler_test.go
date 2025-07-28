package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestReconcilerFunc tests the ReconcilerFunc adapter.
func TestReconcilerFunc(t *testing.T) {
	called := false
	expectedErr := errors.New("test error")

	fn := ReconcilerFunc[*corev1.Pod](func(ctx context.Context, pod *corev1.Pod) error {
		called = true
		if pod.Name != "test-pod" {
			t.Errorf("expected pod name test-pod, got %s", pod.Name)
		}
		return expectedErr
	})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	err := fn.Reconcile(context.Background(), pod)
	if !called {
		t.Error("reconciler function was not called")
	}
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// TestReconcilerInterface tests a struct implementing Reconciler.
type testReconciler struct {
	called bool
	err    error
}

func (r *testReconciler) Reconcile(ctx context.Context, pod *corev1.Pod) error {
	r.called = true
	// Modify the pod to test automatic updates
	if pod.Status.Phase == "" {
		pod.Status.Phase = corev1.PodPending
	}
	return r.err
}

func TestReconcilerInterface(t *testing.T) {
	r := &testReconciler{
		err: errors.New("reconcile error"),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	err := r.Reconcile(context.Background(), pod)
	if !r.called {
		t.Error("reconciler was not called")
	}
	if err != r.err {
		t.Errorf("expected error %v, got %v", r.err, err)
	}
	if pod.Status.Phase != corev1.PodPending {
		t.Errorf("expected pod phase to be set to Pending, got %s", pod.Status.Phase)
	}
}
