package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/generated/valet/public/table"
)

// CreateSession - сохраняет сессию
func (r repository) CreateSession(ctx context.Context, session model.Sessions) error {
	stmt := table.Sessions.INSERT(
		table.Sessions.AllColumns,
	).MODEL(
		session,
	)

	query, args := stmt.Sql()
	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't insert into sessions: %w", err)
	}

	return nil
}

// GetSession - сессия по токену. usecase.ErrNotFound, если такой нет
func (r repository) GetSession(ctx context.Context, token string) (model.Sessions, error) {
	stmt := table.Sessions.SELECT(
		table.Sessions.AllColumns,
	).WHERE(
		table.Sessions.Token.EQ(postgres.String(token)),
	)

	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return model.Sessions{}, fmt.Errorf("can't get session: %w", err)
	}
	session, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[model.Sessions])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Sessions{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Sessions{}, fmt.Errorf("can't scan session: %w", err)
	}

	return session, nil
}

// ExtendSession - переносит конец сессии на expiresAt
func (r repository) ExtendSession(ctx context.Context, token string, expiresAt time.Time) error {
	stmt := table.Sessions.UPDATE(
		table.Sessions.ExpiresAt,
	).SET(
		postgres.TimestampzT(expiresAt),
	).WHERE(
		table.Sessions.Token.EQ(postgres.String(token)),
	)

	query, args := stmt.Sql()
	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't extend session: %w", err)
	}

	return nil
}

// DeleteSession - удаляет сессию по токену
func (r repository) DeleteSession(ctx context.Context, token string) error {
	stmt := table.Sessions.DELETE().WHERE(
		table.Sessions.Token.EQ(postgres.String(token)),
	)

	query, args := stmt.Sql()
	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't delete session: %w", err)
	}

	return nil
}

// DeleteExpiredSessions - удаляет сессии пользователя, истёкшие к now
func (r repository) DeleteExpiredSessions(ctx context.Context, userID uuid.UUID, now time.Time) error {
	stmt := table.Sessions.DELETE().WHERE(
		table.Sessions.UserID.EQ(postgres.UUID(userID)).
			AND(table.Sessions.ExpiresAt.LT_EQ(postgres.TimestampzT(now))),
	)

	query, args := stmt.Sql()
	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't delete expired sessions: %w", err)
	}

	return nil
}
