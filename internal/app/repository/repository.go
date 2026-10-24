package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/generated/valet/public/table"
)

const (
	// codeUniqueViolation - код ошибки postgres «нарушен уникальный индекс»
	codeUniqueViolation = "23505"
	// cardsContentUniqueIdx - у карт пользователя содержимое не повторяется, см. migrations/*_init.sql
	cardsContentUniqueIdx = "cards_user_id_content_unique_idx"
	// usersLoginUniqueIdx - логины не повторяются, см. migrations/*_init.sql
	usersLoginUniqueIdx = "users_login_unique_idx"
)

// isUniqueViolation - база отказалась сохранить строку: она нарушила бы уникальный индекс index
func isUniqueViolation(err error, index string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)

	return ok && pgErr.Code == codeUniqueViolation && pgErr.ConstraintName == index
}

// isCardDuplicate - база отказалась сохранить карту: у пользователя уже есть карта с таким содержимым
func isCardDuplicate(err error) bool {
	return isUniqueViolation(err, cardsContentUniqueIdx)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) repository {
	return repository{
		db: db,
	}
}

// GetUserByLogin - пользователь по логину. usecase.ErrNotFound, если такого нет
func (r repository) GetUserByLogin(ctx context.Context, login string) (model.Users, error) {
	stmt := table.Users.SELECT(
		table.Users.ID,
		table.Users.Login,
		table.Users.Password,
		table.Users.CreatedAt,
	).WHERE(
		table.Users.Login.EQ(postgres.String(login)),
	)

	query, args := stmt.Sql()
	var res model.Users
	err := r.db.QueryRow(ctx, query, args...).Scan(&res.ID, &res.Login, &res.Password, &res.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Users{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Users{}, fmt.Errorf("can't get user: %w", err)
	}

	return res, nil
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
		table.Sessions.Token,
		table.Sessions.UserID,
		table.Sessions.CreatedAt,
		table.Sessions.ExpiresAt,
	).WHERE(
		table.Sessions.Token.EQ(postgres.String(token)),
	)

	query, args := stmt.Sql()
	var res model.Sessions
	err := r.db.QueryRow(ctx, query, args...).Scan(&res.Token, &res.UserID, &res.CreatedAt, &res.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Sessions{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Sessions{}, fmt.Errorf("can't get session: %w", err)
	}

	return res, nil
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

// cardColumns - колонки карты в порядке scanCard
var cardColumns = postgres.ProjectionList{
	table.Cards.ID,
	table.Cards.UserID,
	table.Cards.Name,
	table.Cards.Content,
	table.Cards.Format,
	table.Cards.Note,
	table.Cards.CreatedAt,
	table.Cards.UpdatedAt,
	table.Cards.LastUsedAt,
}

func scanCard(row pgx.Row) (model.Cards, error) {
	var res model.Cards
	err := row.Scan(
		&res.ID,
		&res.UserID,
		&res.Name,
		&res.Content,
		&res.Format,
		&res.Note,
		&res.CreatedAt,
		&res.UpdatedAt,
		&res.LastUsedAt,
	)

	return res, err
}

// ownCard - условие «карта cardID принадлежит userID»: без него любой запрос к карте дырявый
func ownCard(userID uuid.UUID, cardID int64) postgres.BoolExpression {
	return table.Cards.ID.EQ(postgres.Int64(cardID)).
		AND(table.Cards.UserID.EQ(postgres.UUID(userID)))
}

// ListCards - все карты пользователя: недавно открытые сверху
func (r repository) ListCards(ctx context.Context, userID uuid.UUID) ([]model.Cards, error) {
	stmt := table.Cards.SELECT(
		cardColumns,
	).WHERE(
		table.Cards.UserID.EQ(postgres.UUID(userID)),
	).ORDER_BY(
		table.Cards.LastUsedAt.DESC(),
		table.Cards.ID.DESC(),
	)

	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("can't select cards: %w", err)
	}
	defer rows.Close()

	var res []model.Cards
	for rows.Next() {
		card, sErr := scanCard(rows)
		if sErr != nil {
			return nil, fmt.Errorf("can't scan card: %w", sErr)
		}
		res = append(res, card)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error while scanning cards: %w", err)
	}

	return res, nil
}

// GetCard - карта пользователя. usecase.ErrNotFound, если её нет или она чужая
func (r repository) GetCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error) {
	stmt := table.Cards.SELECT(
		cardColumns,
	).WHERE(
		ownCard(userID, cardID),
	)

	query, args := stmt.Sql()
	card, err := scanCard(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Cards{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't get card: %w", err)
	}

	return card, nil
}

// GetCardByContent - карта пользователя с таким содержимым кода. usecase.ErrNotFound, если такой нет
func (r repository) GetCardByContent(ctx context.Context, userID uuid.UUID, content string) (model.Cards, error) {
	stmt := table.Cards.SELECT(
		cardColumns,
	).WHERE(
		table.Cards.UserID.EQ(postgres.UUID(userID)).
			AND(table.Cards.Content.EQ(postgres.String(content))),
	)

	query, args := stmt.Sql()
	card, err := scanCard(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Cards{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't get card by content: %w", err)
	}

	return card, nil
}

// CreateCard - сохраняет карту и возвращает её id. usecase.ErrCardExists, если карта с таким содержимым уже есть
func (r repository) CreateCard(ctx context.Context, card model.Cards) (int64, error) {
	// id выдаёт база
	stmt := table.Cards.INSERT(
		table.Cards.AllColumns.Except(table.Cards.ID),
	).MODEL(
		card,
	).RETURNING(
		table.Cards.ID,
	)

	query, args := stmt.Sql()
	var id int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&id)
	if isCardDuplicate(err) {
		return 0, usecase.ErrCardExists
	}
	if err != nil {
		return 0, fmt.Errorf("can't insert into cards: %w", err)
	}

	return id, nil
}

// UpdateCard - меняет название, содержимое и заметку карты. usecase.ErrNotFound, если её нет или она чужая,
// usecase.ErrCardExists, если новое содержимое совпало с другой картой пользователя
func (r repository) UpdateCard(ctx context.Context, card model.Cards) error {
	stmt := table.Cards.UPDATE(
		table.Cards.Name,
		table.Cards.Content,
		table.Cards.Note,
		table.Cards.UpdatedAt,
	).SET(
		postgres.String(card.Name),
		postgres.String(card.Content),
		postgres.String(card.Note),
		postgres.TimestampzT(card.UpdatedAt),
	).WHERE(
		ownCard(card.UserID, card.ID),
	)

	return r.execOne(ctx, stmt, "can't update card")
}

// TouchCard - отмечает, что карту открыли в at. usecase.ErrNotFound, если её нет или она чужая
func (r repository) TouchCard(ctx context.Context, userID uuid.UUID, cardID int64, at time.Time) error {
	stmt := table.Cards.UPDATE(
		table.Cards.LastUsedAt,
	).SET(
		postgres.TimestampzT(at),
	).WHERE(
		ownCard(userID, cardID),
	)

	return r.execOne(ctx, stmt, "can't touch card")
}

// DeleteCard - удаляет карту. usecase.ErrNotFound, если её нет или она чужая
func (r repository) DeleteCard(ctx context.Context, userID uuid.UUID, cardID int64) error {
	stmt := table.Cards.DELETE().WHERE(
		ownCard(userID, cardID),
	)

	return r.execOne(ctx, stmt, "can't delete card")
}

// execOne - выполняет запрос, который должен задеть ровно одну карту. usecase.ErrNotFound, если не задел ни одной,
// usecase.ErrCardExists, если после запроса у пользователя оказались бы две карты с одним содержимым
func (r repository) execOne(ctx context.Context, stmt postgres.Statement, errMsg string) error {
	query, args := stmt.Sql()
	tag, err := r.db.Exec(ctx, query, args...)
	if isCardDuplicate(err) {
		return usecase.ErrCardExists
	}
	if err != nil {
		return fmt.Errorf("%s: %w", errMsg, err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}

	return nil
}
