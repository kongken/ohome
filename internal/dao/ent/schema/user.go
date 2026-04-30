package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(40).
			Immutable().
			Unique(),
		field.String("username").
			MaxLen(64).
			Unique(),
		field.String("email").
			MaxLen(254).
			Unique(),
		field.String("password_hash").
			Sensitive().
			MaxLen(255),
		field.String("display_name").
			MaxLen(128).
			Optional(),
		field.String("title").
			MaxLen(128).
			Optional(),
		field.Text("bio").
			Optional(),
		field.String("avatar_url").
			Optional(),
		field.String("cover_url").
			Optional(),
		field.String("location").
			MaxLen(128).
			Optional(),
		field.JSON("interests", []string{}).
			Optional(),
		field.Bool("email_verified").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		// Outgoing follows: this user follows others.
		edge.To("following", User.Type).
			From("followers"),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
	}
}
