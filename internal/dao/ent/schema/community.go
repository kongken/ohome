package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Community holds the schema definition for communities. There is no
// create-community endpoint in api.md §6 yet; rows are seeded/admin-managed,
// so IDs may be slugs (e.g. "c_design_systems") or UUIDs.
type Community struct {
	ent.Schema
}

func (Community) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(64).
			Immutable().
			Unique(),
		field.String("name").
			MaxLen(128),
		field.Text("description").
			Optional(),
		field.String("icon_url").
			Optional(),
		field.String("cover_url").
			Optional(),
		field.String("category").
			MaxLen(64).
			Optional(),
		field.Int("members_count").
			Default(0).
			NonNegative(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Community) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("category"),
		index.Fields("members_count"),
		index.Fields("created_at"),
	}
}
