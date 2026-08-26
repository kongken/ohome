package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CommentLike is the join table tracking which users liked which comments.
type CommentLike struct {
	ent.Schema
}

func (CommentLike) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("comment_id").
			MaxLen(40),
		field.String("user_id").
			MaxLen(40),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (CommentLike) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("comment_id", "user_id").Unique(),
		index.Fields("comment_id", "created_at"),
		index.Fields("user_id", "created_at"),
	}
}
