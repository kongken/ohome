package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Membership is the join table between users and communities.
type Membership struct {
	ent.Schema
}

func (Membership) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("user_id").
			MaxLen(40),
		field.String("community_id").
			MaxLen(64),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Membership) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "community_id").Unique(),
		index.Fields("community_id", "created_at"),
		index.Fields("user_id", "created_at"),
	}
}
