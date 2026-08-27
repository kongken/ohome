package dao

import (
	"context"
	"time"

	bmongo "butterfly.orx.me/core/store/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

// EnsureNotificationIndexes creates the notifications indexes at startup:
//   - (user_id ASC, created_at DESC): list pagination + unread lookups
//   - read_at TTL: documents are deleted 30 days after being marked read;
//     unread documents never expire
func EnsureNotificationIndexes(ctx context.Context) error {
	_, err := NotificationsColl().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "read_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32((30 * 24 * time.Hour).Seconds())),
		},
	})
	return err
}
