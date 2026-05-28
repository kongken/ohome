// Package dao is the persistence layer. It wraps an ent client backed by a
// butterfly-managed Postgres connection. Other internal packages (auth,
// users, posts, ...) take the `*ent.Client` exposed by `Client()` and
// never touch the database driver directly.
package dao

import (
	"fmt"

	"butterfly.orx.me/core/store/sqldb"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/kongken/ohome/internal/dao/ent"
)

// DefaultDB is the butterfly `store.db.<name>` key for the primary Postgres
// instance. Configure under `store.db.default` in config.yaml.
const DefaultDB = "default"

var (
	entClient *ent.Client
)

// Init opens the ent client backed by the butterfly-managed Postgres pool.
// Call this from a butterfly InitFunc after butterfly's own store
// initialization has run.
func Init() error {
	db := sqldb.GetDB(DefaultDB)
	if db == nil {
		return fmt.Errorf("dao: butterfly sqldb %q not configured (check store.db.%s in config.yaml)", DefaultDB, DefaultDB)
	}
	drv := entsql.OpenDB("postgres", db)
	entClient = ent.NewClient(ent.Driver(drv))
	return nil
}

// Client returns the application-wide ent client. Returns nil before Init.
func Client() *ent.Client {
	return entClient
}

// Close releases the ent client. The underlying *sql.DB is owned by
// butterfly core and is closed by it on shutdown.
func Close() error {
	if entClient == nil {
		return nil
	}
	err := entClient.Close()
	entClient = nil
	return err
}
