package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SiddheshNan/moonshed/internal/secret"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db  *sql.DB
	box *secret.Box
}

func Open(path string, box *secret.Box) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, box: box}
	if err := checkSchema(db, path); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// checkSchema refuses databases from development builds that used numeric ids.
func checkSchema(db *sql.DB, path string) error {
	var idType string
	err := db.QueryRow("SELECT type FROM pragma_table_info('apps') WHERE name = 'id'").Scan(&idType)
	if err == sql.ErrNoRows || idType == "TEXT" {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%s was created by an older development build; delete the runtime's data folder to start fresh", path)
}

// NewID returns a UUIDv7.
func NewID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// nullID stores an empty id as NULL, for optional foreign keys.
func nullID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// OpenDB opens a SQLite file with the pragmas every DB in the platform uses.
// A single connection serializes writes and avoids SQLITE_BUSY entirely.
func OpenDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB, steps []string) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(steps); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(steps[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Migrate is exported for other databases (metrics) that share the same mechanism.
func Migrate(db *sql.DB, steps []string) error { return migrate(db, steps) }

// Time is stored as unix seconds and serialized as null when unset.
type Time struct{ time.Time }

func Now() Time { return Time{time.Now().Truncate(time.Second)} }

func (t *Time) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		t.Time = time.Time{}
	case int64:
		t.Time = time.Unix(x, 0)
	default:
		return fmt.Errorf("store.Time: unsupported type %T", v)
	}
	return nil
}

func (t Time) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Unix(), nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time)
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
