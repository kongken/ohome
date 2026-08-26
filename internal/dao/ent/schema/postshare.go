package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PostShare marks that a user has shared a post at least once (for the
// viewer.shared flag). The shares counter on Post increments per share
// event and is not tied to this table's uniqueness.
type PostShare struct {
	ent.Schema
}

func (PostShare) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("post_id").
			MaxLen(40),
		field.String("user_id").
			MaxLen(40),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (PostShare) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id", "user_id").Unique(),
		index.Fields("user_id", "created_at"),
	}
}
