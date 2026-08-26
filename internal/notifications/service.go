package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/kongken/ohome/internal/dao"
)

// Input describes an event worth notifying someone about.
type Input struct {
	UserID       string // recipient (required); self-actions are skipped
	Type         string // one of the Type* constants (required)
	ActorID      string // who triggered it (optional for system events)
	PostID       string
	TargetUserID string
	Text         string // optional override of the per-type default
	Payload      map[string]any
}

// Create persists a notification document and returns its ID. Self-actions
// (actor == recipient) are silently skipped with an empty ID.
func Create(ctx context.Context, in Input) (string, error) {
	if in.UserID == "" {
		return "", fmt.Errorf("notification: user_id is required")
	}
	if !ValidType(in.Type) {
		return "", fmt.Errorf("notification: unknown type %q", in.Type)
	}
	if in.ActorID == in.UserID {
		return "", nil // never notify people about their own actions
	}

	text := in.Text
	if text == "" {
		text = DefaultText(in.Type)
	}

	doc := Notification{
		ID:           uuid.NewString(),
		UserID:       in.UserID,
		Type:         in.Type,
		ActorID:      in.ActorID,
		PostID:       in.PostID,
		TargetUserID: in.TargetUserID,
		Text:         text,
		CreatedAt:    time.Now(),
	}
	if len(in.Payload) > 0 {
		doc.Payload = bsonFromAny(in.Payload)
	}

	res, err := dao.NotificationsColl().InsertOne(ctx, doc)
	if err != nil {
		return "", fmt.Errorf("notification insert: %w", err)
	}
	if id, ok := res.InsertedID.(string); ok {
		return id, nil
	}
	return doc.ID, nil
}

// TryNotify is the best-effort entry point used by primary flows (likes,
// comments, follows, ...): notification failures are logged, never propagated.
func TryNotify(ctx context.Context, in Input) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := Create(ctx, in); err != nil {
		slog.Warn("notification skipped",
			"type", in.Type, "user_id", in.UserID, "actor_id", in.ActorID, "error", err)
	}
}

// bsonFromAny shallow-converts a map[string]any payload into bson.M.
func bsonFromAny(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
