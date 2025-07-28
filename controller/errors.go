package controller

import (
	"errors"
	"fmt"
	"time"
)

// requeueAfter indicates the reconciler should requeue the item after a duration.
type requeueAfter struct {
	duration time.Duration
}

func (r *requeueAfter) Error() string {
	return fmt.Sprintf("requeue after %v", r.duration)
}

// RequeueAfter returns an error that indicates the reconciler should requeue
// the item after the specified duration.
func RequeueAfter(d time.Duration) error {
	return &requeueAfter{duration: d}
}

// requeueImmediately indicates the reconciler should requeue the item immediately.
type requeueImmediately struct{}

func (r *requeueImmediately) Error() string {
	return "requeue immediately"
}

// RequeueImmediately returns an error that indicates the reconciler should
// requeue the item immediately.
func RequeueImmediately() error {
	return &requeueImmediately{}
}

// permanentError wraps an error to indicate it should not be retried.
type permanentError struct {
	err error
}

func (p *permanentError) Error() string {
	return fmt.Sprintf("permanent error: %v", p.err)
}

func (p *permanentError) Unwrap() error {
	return p.err
}

// Is implements error matching for permanentError
func (p *permanentError) Is(target error) bool {
	return errors.Is(target, &permanentError{})
}

// PermanentError wraps an error to indicate that it should not be retried.
// The reconciler will not requeue the item.
func PermanentError(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanentError checks if an error is a permanent error.
func IsPermanentError(err error) bool {
	return errors.Is(err, &permanentError{})
}

// IsRequeueError checks if an error is a requeue error.
func IsRequeueError(err error) bool {
	return errors.Is(err, &requeueAfter{}) ||
		errors.Is(err, &requeueImmediately{})
}

// GetRequeueDuration returns the requeue duration if the error indicates
// a requeue after duration, otherwise returns 0.
func GetRequeueDuration(err error) time.Duration {
	var ra *requeueAfter
	if errors.As(err, &ra) {
		return ra.duration
	}
	return 0
}
