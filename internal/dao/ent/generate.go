// Package ent holds the ent ORM client and schema for ohome.
//
// To regenerate the typed client after editing schema files:
//
//	make ent
//
// or directly:
//
//	go generate ./internal/dao/ent
package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --target . ./schema
