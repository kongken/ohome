package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Bookmark is the join table for saved posts (sidebar Bookmarks list).
type Bookmark struct {
	ent.Schema
}

func (Bookmark) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("user_id").
			MaxLen(40),
		field.String("post_id").
			MaxLen(40),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Bookmark) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "post_id").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("post_id"),
	}
}
