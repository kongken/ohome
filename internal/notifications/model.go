package notifications

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Notification types (api.md §8). Kept as plain strings in documents so new
// types can be added without a migration.
const (
	TypeLike              = "like"
	TypeComment           = "comment"
	TypeMention           = "mention"
	TypeFollow            = "follow"
	TypeConnectionRequest = "connection_request"
	TypeCommunityInvite   = "community_invite"
	TypePostShare         = "post_share"
	TypeSystem            = "system"
)

// ValidType reports whether t is a known notification type.
func ValidType(t string) bool {
	switch t {
	case TypeLike, TypeComment, TypeMention, TypeFollow,
		TypeConnectionRequest, TypeCommunityInvite, TypePostShare, TypeSystem:
		return true
	default:
		return false
	}
}

// DefaultText is the fallback display text per type when the caller does not
// supply one. Stored at write time so reads stay render-free.
func DefaultText(t string) string {
	switch t {
	case TypeLike:
		return "liked your post"
	case TypeComment:
		return "commented on your post"
	case TypeMention:
		return "mentioned you"
	case TypeFollow:
		return "started following you"
	case TypeConnectionRequest:
		return "sent you a connection request"
	case TypeCommunityInvite:
		return "invited you to a community"
	case TypePostShare:
		return "shared your post"
	default:
		return ""
	}
}

// Notification is the stored document shape (task.md storage map):
//
//	{_id, user_id, type, actor_id, target_*, payload, created_at, read_at}
type Notification struct {
	ID           string     `bson:"_id"`
	UserID       string     `bson:"user_id"` // recipient
	Type         string     `bson:"type"`
	ActorID      string     `bson:"actor_id,omitempty"`
	PostID       string     `bson:"post_id,omitempty"`
	TargetUserID string     `bson:"target_user_id,omitempty"`
	Text         string     `bson:"text,omitempty"`
	Payload      bson.M     `bson:"payload,omitempty"`
	CreatedAt    time.Time  `bson:"created_at"`
	ReadAt       *time.Time `bson:"read_at,omitempty"`
}
