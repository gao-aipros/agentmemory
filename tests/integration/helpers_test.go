package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	migrationCache     []migrationFile
	migrationCacheOnce sync.Once
)

type migrationFile struct {
	Name string
	SQL  string
}

// RunAllMigrations reads all .up.sql migration files from the migrations/ directory
// (relative to repo root) and executes them in order against the given pool.
// This ensures ALL integration tests use the same migration content, including
// PL/pgSQL wrapper functions for ParadeDB (bm25_search, hybrid_search).
func RunAllMigrations(pool *pgxpool.Pool) error {
	migrationCacheOnce.Do(func() {
		loadMigrations()
	})
	return executeMigrations(pool, migrationCache)
}

func loadMigrations() {
	// Find the repo root by walking up from the test file location
	dir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("cannot get working directory: %v", err))
	}

	// Walk up to find migrations/ directory
	for {
		migrationsDir := filepath.Join(dir, "migrations")
		if info, err := os.Stat(migrationsDir); err == nil && info.IsDir() {
			entries, err := os.ReadDir(migrationsDir)
			if err != nil {
				panic(fmt.Sprintf("cannot read migrations directory: %v", err))
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
					content, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
					if err != nil {
						panic(fmt.Sprintf("cannot read migration %s: %v", entry.Name(), err))
					}
					migrationCache = append(migrationCache, migrationFile{
						Name: entry.Name(),
						SQL:  string(content),
					})
				}
			}
			sort.Slice(migrationCache, func(i, j int) bool {
				return migrationCache[i].Name < migrationCache[j].Name
			})
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("cannot find migrations/ directory")
		}
		dir = parent
	}
}

func executeMigrations(pool *pgxpool.Pool, migrations []migrationFile) error {
	ctx := context.Background()
	for _, m := range migrations {
		if _, err := pool.Exec(ctx, m.SQL); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.Name, err)
		}
	}
	return nil
}

// RunMigrations is an alias for RunAllMigrations for backward compatibility.
func RunMigrations(pool *pgxpool.Pool) error {
	return RunAllMigrations(pool)
}
