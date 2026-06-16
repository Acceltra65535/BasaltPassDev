/*
 * Copyright (c) 2025.  Zeturn. Contains AI-generated content, reviewed by Zeturn.
 */

package common

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"basaltpass-backend/internal/config"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db    *gorm.DB
	dbErr error
	dbMu  sync.Mutex
)

// DB returns a singleton *gorm.DB connected using configured driver.
// Keep this package decoupled from internal/model to avoid import cycles.
func DB() *gorm.DB {
	dbMu.Lock()
	defer dbMu.Unlock()

	if db != nil {
		return db
	}

	cfg := config.Get()
	driver := cfg.Database.Driver
	var err error

	switch driver {
	case "mysql":
		dsn := cfg.Database.DSN
		log.Printf("Database driver: mysql")
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			DisableForeignKeyConstraintWhenMigrating: true, // 防止复杂的循环依赖导致迁移失败
		})
	case "postgres", "postgresql":
		dsn := cfg.Database.DSN
		log.Printf("Database driver: postgres")
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			DisableForeignKeyConstraintWhenMigrating: true,
		})
	case "sqlite", "sqlite3", "":
		// Prefer DSN when provided; else build from path relative to Project Root (if found) or CWD
		dsn := cfg.Database.DSN
		if dsn == "" {
			wd, wdErr := os.Getwd()
			if wdErr != nil {
				err = fmt.Errorf("failed to get working directory: %w", wdErr)
				break
			}

			// Attempt to find project root by looking for go.mod
			projectRoot := wd
			for {
				if _, statErr := os.Stat(filepath.Join(projectRoot, "go.mod")); statErr == nil {
					break
				}
				parent := filepath.Dir(projectRoot)
				if parent == projectRoot {
					projectRoot = wd // fallback to original wd if not found
					break
				}
				projectRoot = parent
			}

			dbPath := cfg.Database.Path
			if dbPath == "" {
				dbPath = "basaltpass.db"
			}
			dsn = filepath.Join(projectRoot, dbPath)
		}
		log.Printf("Database driver: sqlite")
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	default:
		err = fmt.Errorf("unsupported database driver: %s", driver)
	}

	if err != nil {
		dbErr = err
		db = nil
		log.Printf("failed to connect database: %v", err)
		return nil
	}

	dbErr = nil
	return db
}

// DBErr returns the last database initialization error, if any.
func DBErr() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	return dbErr
}

// DBReady reports whether the database is reachable right now.
func DBReady() bool {
	dbMu.Lock()
	current := db
	dbMu.Unlock()
	if current == nil {
		current = DB()
		if current == nil {
			return false
		}
	}
	sqlDB, err := current.DB()
	if err != nil {
		return false
	}
	if err := sqlDB.Ping(); err != nil {
		return false
	}
	return true
}

// SetDBForTest injects a custom database connection for tests.
func SetDBForTest(testDB *gorm.DB) {
	dbMu.Lock()
	defer dbMu.Unlock()
	db = testDB
	dbErr = nil
}
