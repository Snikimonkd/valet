package testsupport

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"valet/internal/generated/valet/public/model"
	"valet/internal/generated/valet/public/table"
)

// InsertUser - пользователь со случайным логином. После теста удаляется вместе со своими сессиями и картами
func InsertUser(ctx context.Context, t *testing.T, db *pgxpool.Pool, passwordHash string) model.Users {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("can't generate user id: %v", err)
	}

	stmt := table.Users.INSERT(
		table.Users.ID, table.Users.Login, table.Users.Password,
	).VALUES(
		id, gofakeit.Username()+"-"+gofakeit.UUID(), passwordHash,
	).RETURNING(
		table.Users.AllColumns,
	)

	query, args := stmt.Sql()
	var user model.Users
	err = db.QueryRow(ctx, query, args...).Scan(&user.ID, &user.Login, &user.Password, &user.CreatedAt)
	if err != nil {
		t.Fatalf("can't insert user: %v", err)
	}

	// внешних ключей нет, каскада тоже: карты и сессии пользователя убираем сами
	t.Cleanup(func() {
		ctx := context.WithoutCancel(ctx)
		for _, del := range []postgres.DeleteStatement{
			table.Cards.DELETE().WHERE(table.Cards.UserID.EQ(postgres.UUID(user.ID))),
			table.Sessions.DELETE().WHERE(table.Sessions.UserID.EQ(postgres.UUID(user.ID))),
			table.Users.DELETE().WHERE(table.Users.ID.EQ(postgres.UUID(user.ID))),
		} {
			query, args := del.Sql()
			_, err := db.Exec(ctx, query, args...)
			if err != nil {
				t.Errorf("can't delete user data: %v", err)
			}
		}
	})

	return user
}

// FakeCard - карта пользователя userID со случайными названием и номером, ещё не сохранённая
func FakeCard(userID uuid.UUID) model.Cards {
	now := time.Now().UTC().Truncate(time.Microsecond)

	return model.Cards{
		UserID:     userID,
		Name:       gofakeit.Company(),
		Content:    gofakeit.Numerify("#############"),
		Format:     "qr",
		Note:       gofakeit.Sentence(3),
		CreatedAt:  now,
		UpdatedAt:  now,
		LastUsedAt: now,
	}
}
