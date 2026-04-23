package service

import (
	"testing"

	"github.com/google/uuid"
)

func TestAuditLog(t *testing.T) {
	// AuditLog should not panic.
	AuditLog("test_action", uuid.New(), uuid.New(), "target-123", "test detail")
}
