package service

import (
	"log/slog"

	"github.com/google/uuid"
)

// AuditLog emits a structured log line for sensitive operations.
// Fields: action (what happened), actorID (who did it), orgID (which org),
// targetID (affected resource), detail (extra context).
func AuditLog(action string, actorID, orgID uuid.UUID, targetID, detail string) {
	slog.Info("audit",
		"action", action,
		"actor", actorID,
		"org", orgID,
		"target", targetID,
		"detail", detail,
	)
}
