package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestGetOwnerReference(t *testing.T) {
	// Create a test ConfigMap
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	ref, err := GetOwnerReference(cm, scheme.Scheme)
	if err != nil {
		t.Fatalf("failed to get owner reference: %v", err)
	}

	if ref.APIVersion != "v1" {
		t.Errorf("expected APIVersion v1, got %s", ref.APIVersion)
	}
	if ref.Kind != "ConfigMap" {
		t.Errorf("expected Kind ConfigMap, got %s", ref.Kind)
	}
	if ref.Name != "test-cm" {
		t.Errorf("expected Name test-cm, got %s", ref.Name)
	}
	if ref.UID != "test-uid" {
		t.Errorf("expected UID test-uid, got %s", ref.UID)
	}
}

func TestSetOwnerReference(t *testing.T) {
	owner := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-cm",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	owned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned-secret",
			Namespace: "default",
		},
	}

	// Set non-controller reference
	err := SetOwnerReference(owned, owner, scheme.Scheme, false)
	if err != nil {
		t.Fatalf("failed to set owner reference: %v", err)
	}

	refs := owned.GetOwnerReferences()
	if len(refs) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(refs))
	}

	ref := refs[0]
	if ref.Controller != nil && *ref.Controller {
		t.Error("expected non-controller reference")
	}

	// Set controller reference
	if err := SetOwnerReference(owned, owner, scheme.Scheme, true); err != nil {
		t.Fatalf("failed to set controller reference: %v", err)
	}

	refs = owned.GetOwnerReferences()
	if len(refs) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(refs))
	}

	ref = refs[0]
	if ref.Controller == nil || !*ref.Controller {
		t.Error("expected controller reference")
	}
}

func TestIsOwnedBy(t *testing.T) {
	owned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned-secret",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "owner-cm",
					UID:        "owner-uid",
				},
			},
		},
	}

	if !IsOwnedBy(owned, "owner-uid") {
		t.Error("expected object to be owned by owner-uid")
	}

	if IsOwnedBy(owned, "other-uid") {
		t.Error("expected object not to be owned by other-uid")
	}
}

func TestGetControllerReference(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name     string
		owned    metav1.Object
		expected bool
	}{
		{
			name: "has controller reference",
			owned: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner1",
							UID:        "uid1",
							Controller: &boolFalse,
						},
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner2",
							UID:        "uid2",
							Controller: &boolTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "no controller reference",
			owned: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner1",
							UID:        "uid1",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "no owner references",
			owned: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := GetControllerReference(tt.owned)
			if tt.expected && ref == nil {
				t.Error("expected controller reference, got nil")
			}
			if !tt.expected && ref != nil {
				t.Error("expected no controller reference, got one")
			}
			if ref != nil && ref.Name != "owner2" {
				t.Errorf("expected controller reference to be owner2, got %s", ref.Name)
			}
		})
	}
}

func TestEnqueueRequestForOwner(t *testing.T) {
	ownerGVK := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}

	tests := []struct {
		name         string
		obj          runtime.Object
		isController bool
		expected     []string
	}{
		{
			name: "owned by matching type",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret1",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner-cm",
						},
					},
				},
			},
			isController: false,
			expected:     []string{"default/owner-cm"},
		},
		{
			name: "owned by multiple matching types",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret1",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner-cm1",
						},
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner-cm2",
						},
					},
				},
			},
			isController: false,
			expected:     []string{"default/owner-cm1", "default/owner-cm2"},
		},
		{
			name: "controller reference only",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret1",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner-cm1",
						},
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "owner-cm2",
							Controller: &[]bool{true}[0],
						},
					},
				},
			},
			isController: true,
			expected:     []string{"default/owner-cm2"},
		},
		{
			name: "no matching owner",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret1",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "owner-deploy",
						},
					},
				},
			},
			isController: false,
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := EnqueueRequestForOwner[*corev1.ConfigMap, runtime.Object](ownerGVK, tt.isController)
			requests := handler(tt.obj)

			if len(requests) != len(tt.expected) {
				t.Errorf("expected %d requests, got %d", len(tt.expected), len(requests))
				return
			}

			for i, req := range requests {
				if req != tt.expected[i] {
					t.Errorf("expected request %s, got %s", tt.expected[i], req)
				}
			}
		})
	}
}

// Integration test with mocked controller
type mockQueue struct {
	items []string
}

func (q *mockQueue) Add(item string)                              { q.items = append(q.items, item) }
func (q *mockQueue) Len() int                                     { return len(q.items) }
func (q *mockQueue) Get() (string, bool)                          { return "", false }
func (q *mockQueue) Done(item string)                             {}
func (q *mockQueue) ShutDown()                                    {}
func (q *mockQueue) ShutDownWithDrain()                           {}
func (q *mockQueue) ShuttingDown() bool                           { return false }
func (q *mockQueue) AddAfter(item string, duration time.Duration) {}
func (q *mockQueue) AddRateLimited(item string)                   {}
func (q *mockQueue) Forget(item string)                           {}
func (q *mockQueue) NumRequeues(item string) int                  { return 0 }

func TestControllerEnqueueOwners(t *testing.T) {
	// Create a mock controller
	queue := &mockQueue{}
	ctrl := &Controller[*corev1.ConfigMap]{
		queue: queue,
	}

	ownerGVK := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}

	// Test enqueuing owners
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "owner-cm",
				},
			},
		},
	}

	ctrl.enqueueOwners(context.Background(), secret, ownerGVK, false)

	if len(queue.items) != 1 {
		t.Errorf("expected 1 item in queue, got %d", len(queue.items))
	}

	if queue.items[0] != "default/owner-cm" {
		t.Errorf("expected default/owner-cm in queue, got %v", queue.items[0])
	}
}
