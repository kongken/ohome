package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Comment holds the schema definition for threaded post comments.
// Replies reference their parent via parent_id (one nesting level:
// replies to replies are flattened onto the top-level parent).
type Comment struct {
	ent.Schema
}

func (Comment) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("post_id").
			MaxLen(40),
		field.String("author_id").
			MaxLen(40),
		field.String("parent_id").
			MaxLen(40).
			Optional(),
		field.Text("content"),
		field.Int("likes_count").
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

func (Comment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id", "created_at"),
		index.Fields("parent_id", "created_at"),
		index.Fields("author_id", "created_at"),
	}
}
