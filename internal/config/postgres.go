package config

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	retries      int           = 3
	retryTimeout time.Duration = time.Second * 5
)

// ConnectToPostgres - подключается к postgres по dsn. База может подниматься дольше приложения,
// поэтому до retries попыток с паузой retryTimeout
func ConnectToPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("can't connect to db: %w", err)
	}

	for attempt := range retries {
		err := db.Ping(ctx)
		if err == nil {
			return db, nil
		}

		slog.WarnContext(ctx, "can't ping db", "err", err, "attempt", attempt+1, "retries", retries)
		time.Sleep(retryTimeout)
	}

	db.Close()

	return nil, fmt.Errorf("failed to ping db after %v retries", retries)
}
