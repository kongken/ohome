package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Post holds the schema definition for social feed posts.
type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("author_id").
			MaxLen(40),
		field.Text("content"),
		field.JSON("attachments", []PostAttachment{}).
			Optional(),
		field.JSON("hashtags", []string{}).
			Optional(),
		field.String("community_id").
			MaxLen(64).
			Optional(),
		field.String("event_id").
			MaxLen(64).
			Optional(),
		field.String("visibility").
			Default("public").
			MaxLen(32),
		field.Int("likes_count").
			Default(0).
			NonNegative(),
		field.Int("comments_count").
			Default(0).
			NonNegative(),
		field.Int("shares_count").
			Default(0).
			NonNegative(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.Time("deleted_at").
			Optional().
			Nillable(),
	}
}

func (Post) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("author_id", "created_at"),
		index.Fields("community_id", "created_at"),
		index.Fields("visibility", "created_at"),
	}
}

// PostAttachment is persisted as JSON on Post until the media table exists.
type PostAttachment struct {
	Type    string `json:"type,omitempty"`
	MediaID string `json:"media_id,omitempty"`
	URL     string `json:"url,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}
