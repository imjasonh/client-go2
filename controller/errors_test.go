package controller

import (
	"errors"
	"testing"
	"time"
)

func TestRequeueAfter(t *testing.T) {
	duration := 5 * time.Second
	err := RequeueAfter(duration)

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !IsRequeueError(err) {
		t.Error("expected IsRequeueError to return true")
	}

	if IsPermanentError(err) {
		t.Error("expected IsPermanentError to return false")
	}

	if d := GetRequeueDuration(err); d != duration {
		t.Errorf("expected duration %v, got %v", duration, d)
	}

	expectedMsg := "requeue after 5s"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestRequeueImmediately(t *testing.T) {
	err := RequeueImmediately()

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !IsRequeueError(err) {
		t.Error("expected IsRequeueError to return true")
	}

	if IsPermanentError(err) {
		t.Error("expected IsPermanentError to return false")
	}

	if d := GetRequeueDuration(err); d != 0 {
		t.Errorf("expected duration 0, got %v", d)
	}

	expectedMsg := "requeue immediately"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestPermanentError(t *testing.T) {
	baseErr := errors.New("base error")
	err := PermanentError(baseErr)

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !IsPermanentError(err) {
		t.Error("expected IsPermanentError to return true")
	}

	if IsRequeueError(err) {
		t.Error("expected IsRequeueError to return false")
	}

	expectedMsg := "permanent error: base error"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}

	// Test unwrap
	var unwrapped *permanentError
	if !errors.As(err, &unwrapped) {
		t.Error("expected error to be unwrappable to *permanentError")
	}
	if errors.Unwrap(err) != baseErr {
		t.Error("expected unwrapped error to be baseErr")
	}
}

func TestPermanentErrorNil(t *testing.T) {
	err := PermanentError(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestGetRequeueDuration(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{
			name:     "requeue after 10s",
			err:      RequeueAfter(10 * time.Second),
			expected: 10 * time.Second,
		},
		{
			name:     "requeue immediately",
			err:      RequeueImmediately(),
			expected: 0,
		},
		{
			name:     "permanent error",
			err:      PermanentError(errors.New("test")),
			expected: 0,
		},
		{
			name:     "regular error",
			err:      errors.New("regular"),
			expected: 0,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := GetRequeueDuration(tt.err)
			if duration != tt.expected {
				t.Errorf("expected duration %v, got %v", tt.expected, duration)
			}
		})
	}
}
