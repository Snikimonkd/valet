package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/generated/valet/public/table"
)

// usersLoginUniqueIdx - логины не повторяются, см. migrations/*_users.sql
const usersLoginUniqueIdx = "users_login_unique_idx"

// GetUserByLogin - пользователь по логину. usecase.ErrNotFound, если такого нет
func (r repository) GetUserByLogin(ctx context.Context, login string) (model.Users, error) {
	stmt := table.Users.SELECT(
		table.Users.AllColumns,
	).WHERE(
		table.Users.Login.EQ(postgres.String(login)),
	)

	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return model.Users{}, fmt.Errorf("can't get user: %w", err)
	}
	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[model.Users])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Users{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Users{}, fmt.Errorf("can't scan user: %w", err)
	}

	return user, nil
}

// CreateUser - сохраняет пользователя. usecase.ErrLoginTaken, если логин уже занят
func (r repository) CreateUser(ctx context.Context, user model.Users) error {
	stmt := table.Users.INSERT(
		table.Users.AllColumns,
	).MODEL(
		user,
	)

	query, args := stmt.Sql()
	_, err := r.db.Exec(ctx, query, args...)
	if isUniqueViolation(err, usersLoginUniqueIdx) {
		return usecase.ErrLoginTaken
	}
	if err != nil {
		return fmt.Errorf("can't insert into users: %w", err)
	}

	return nil
}
