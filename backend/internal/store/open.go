package store

import (
	"fmt"
	"reflect"
	"time"

	glebarezsqlite "github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	gormschema "gorm.io/gorm/schema"
)

// uuidType is compared against each model's ID field to decide whether the
// UUID-assignment callback applies.
var uuidType = reflect.TypeOf(uuid.UUID{})

func gormConfig(debug bool) *gorm.Config {
	cfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}
	if debug {
		cfg.Logger = gormlogger.Default.LogMode(gormlogger.Info)
	}
	return cfg
}

// OpenPostgres opens a GORM connection to PostgreSQL, retrying while the
// database becomes available, and registers the shared callbacks.
func OpenPostgres(url string, debug bool, logger *zap.Logger) (*gorm.DB, error) {
	if url == "" {
		return nil, fmt.Errorf("DB_URL is required for the postgres storage driver")
	}

	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 10; attempt++ {
		db, err = gorm.Open(postgres.Open(url), gormConfig(debug))
		if err == nil {
			sqlDB, sqlErr := db.DB()
			if sqlErr == nil {
				sqlDB.SetMaxOpenConns(25)
				sqlDB.SetMaxIdleConns(5)
				sqlDB.SetConnMaxLifetime(5 * time.Minute)
				if pingErr := sqlDB.Ping(); pingErr == nil {
					if err := registerCallbacks(db); err != nil {
						return nil, err
					}
					return db, nil
				}
			}
		}
		logger.Warn("waiting for postgres", zap.Int("attempt", attempt), zap.Error(err))
		time.Sleep(3 * time.Second)
	}
	return nil, fmt.Errorf("could not connect to postgres after 10 attempts: %w", err)
}

// OpenSQLite opens a GORM connection to a local SQLite database file using the
// pure-Go driver (no CGO, so the binary stays statically linked and portable).
func OpenSQLite(path string, debug bool, logger *zap.Logger) (*gorm.DB, error) {
	if path == "" {
		path = "kubepilot.db"
	}

	db, err := gorm.Open(glebarezsqlite.Open(path), gormConfig(debug))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite tolerates a single writer; serialise access and let readers wait
	// rather than fail with "database is locked".
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		logger.Warn("set sqlite busy_timeout", zap.Error(err))
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		logger.Warn("set sqlite journal_mode", zap.Error(err))
	}

	if err := registerCallbacks(db); err != nil {
		return nil, err
	}
	logger.Info("using sqlite storage", zap.String("path", path))
	return db, nil
}

// registerCallbacks wires DB-agnostic behaviour. Currently it assigns a v4 UUID
// to any model's zero-valued ID before insert, so we no longer depend on the
// PostgreSQL-only `gen_random_uuid()` column default and work identically on
// SQLite.
func registerCallbacks(db *gorm.DB) error {
	return db.Callback().Create().Before("gorm:create").Register("kubepilot:assign_uuid", assignUUID)
}

func assignUUID(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}
	field := db.Statement.Schema.LookUpField("ID")
	if field == nil || field.FieldType != uuidType {
		return
	}

	switch db.Statement.ReflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < db.Statement.ReflectValue.Len(); i++ {
			assignUUIDToElem(db, field, db.Statement.ReflectValue.Index(i))
		}
	case reflect.Struct:
		assignUUIDToElem(db, field, db.Statement.ReflectValue)
	}
}

func assignUUIDToElem(db *gorm.DB, field *gormschema.Field, rv reflect.Value) {
	if !rv.CanAddr() {
		return
	}
	if _, isZero := field.ValueOf(db.Statement.Context, rv); isZero {
		_ = field.Set(db.Statement.Context, rv, uuid.New())
	}
}
