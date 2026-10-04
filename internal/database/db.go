package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

// Open creates or connects to the SQLite database with WAL mode and proper pragmas.
func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", dbPath)
	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// SQLite handles concurrent reads with WAL, but single writer connection avoids lock contention
	rawDB.SetMaxOpenConns(1)
	rawDB.SetMaxIdleConns(1)

	if err := rawDB.Ping(); err != nil {
		rawDB.Close()
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}

	db := &DB{DB: rawDB}
	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running database migrations: %w", err)
	}

	return db, nil
}
