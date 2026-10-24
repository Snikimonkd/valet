// Package testsupport - общее для интеграционных тестов: подключение к тестовой базе и фикстуры
package testsupport

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"valet/internal/config"
)

var (
	db         *pgxpool.Pool
	errConnect error
	dbOnce     sync.Once
)

// defaultDSN - база для тестов, если PG_DSN не задан: та же, что поднимает make postgres-up
const defaultDSN string = `postgres://postgres@localhost:5432/valet?sslmode=disable`

// ConnectToTestPostgres - подключение к тестовой базе, одно на все тесты пакета
func ConnectToTestPostgres(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbOnce.Do(func() {
		dsn := os.Getenv("PG_DSN")
		if dsn == "" {
			dsn = defaultDSN
		}
		db, errConnect = config.ConnectToPostgres(ctx, dsn)
	})
	if errConnect != nil {
		t.Fatalf("can't connect to db: %v", errConnect)
	}

	return db
}
