package service

import (
	"testing"
)

func TestServiceError_Error(t *testing.T) {
	e := &ServiceError{Code: "test_error", Message: "something broke", Status: 400}
	if e.Error() != "something broke" {
		t.Errorf("Error() = %q, want %q", e.Error(), "something broke")
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Code: "invalid_name", Message: "name is required"}
	if e.Error() != "name is required" {
		t.Errorf("Error() = %q, want %q", e.Error(), "name is required")
	}
}

func TestSpanQuotaExceededError_Error(t *testing.T) {
	e := &SpanQuotaExceededError{}
	if e.Error() != "span quota exceeded" {
		t.Errorf("Error() = %q, want %q", e.Error(), "span quota exceeded")
	}
}
