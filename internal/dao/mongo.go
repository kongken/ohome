package dao

import (
	bmongo "butterfly.orx.me/core/store/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoDB returns the named butterfly-managed mongo client. Configure under
// `store.mongo.<name>` in config.yaml.
//
// Conventions in ohome:
//   - "default" — primary cluster holding `notifications` and `messages`
//     collections. See task.md for the per-entity storage map.
func MongoDB(name string) *mongo.Client {
	return bmongo.GetClient(name)
}

// NotificationsColl returns the notifications collection on the default
// mongo client. Documents are TTL-indexed on `read_at`.
func NotificationsColl() *mongo.Collection {
	return MongoDB("default").Database("ohome").Collection("notifications")
}

// MessagesColl returns the messages collection. Sharded / partitioned by
// `conversation_id`.
func MessagesColl() *mongo.Collection {
	return MongoDB("default").Database("ohome").Collection("messages")
}

// ConversationsColl stores conversation metadata (participants, last_message,
// per-user unread counters).
func ConversationsColl() *mongo.Collection {
	return MongoDB("default").Database("ohome").Collection("conversations")
}
