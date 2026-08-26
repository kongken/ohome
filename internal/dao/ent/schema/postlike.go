package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PostLike is the join table tracking which users liked which posts.
type PostLike struct {
	ent.Schema
}

func (PostLike) Fields() []ent.Field {
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

func (PostLike) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id", "user_id").Unique(),
		index.Fields("post_id", "created_at"),
		index.Fields("user_id", "created_at"),
	}
}
