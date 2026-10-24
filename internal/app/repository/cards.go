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

// cardsContentUniqueIdx - у карт пользователя содержимое не повторяется, см. migrations/*_cards.sql
const cardsContentUniqueIdx = "cards_user_id_content_unique_idx"

// isCardDuplicate - база отказалась сохранить карту: у пользователя уже есть карта с таким содержимым
func isCardDuplicate(err error) bool {
	return isUniqueViolation(err, cardsContentUniqueIdx)
}

// ListCards - все карты пользователя: недавно открытые сверху
func (r repository) ListCards(ctx context.Context, userID uuid.UUID) ([]model.Cards, error) {
	stmt := table.Cards.SELECT(
		table.Cards.AllColumns,
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
	cards, err := pgx.CollectRows(rows, pgx.RowToStructByPos[model.Cards])
	if err != nil {
		return nil, fmt.Errorf("can't scan cards: %w", err)
	}

	return cards, nil
}

// GetCard - карта пользователя. usecase.ErrNotFound, если её нет или она чужая
func (r repository) GetCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error) {
	stmt := table.Cards.SELECT(
		table.Cards.AllColumns,
	).WHERE(
		table.Cards.ID.EQ(postgres.Int64(cardID)).
			AND(table.Cards.UserID.EQ(postgres.UUID(userID))),
	)

	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't get card: %w", err)
	}
	card, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[model.Cards])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Cards{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't scan card: %w", err)
	}

	return card, nil
}

// GetCardByContent - карта пользователя с таким содержимым кода. usecase.ErrNotFound, если такой нет
func (r repository) GetCardByContent(ctx context.Context, userID uuid.UUID, content string) (model.Cards, error) {
	stmt := table.Cards.SELECT(
		table.Cards.AllColumns,
	).WHERE(
		table.Cards.UserID.EQ(postgres.UUID(userID)).
			AND(table.Cards.Content.EQ(postgres.String(content))),
	)

	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't get card by content: %w", err)
	}
	card, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[model.Cards])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Cards{}, usecase.ErrNotFound
	}
	if err != nil {
		return model.Cards{}, fmt.Errorf("can't scan card by content: %w", err)
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
		table.Cards.ID.EQ(postgres.Int64(card.ID)).
			AND(table.Cards.UserID.EQ(postgres.UUID(card.UserID))),
	)

	// новое содержимое может совпасть с другой картой пользователя: тогда упрёмся в уникальный индекс
	query, args := stmt.Sql()
	tag, err := r.db.Exec(ctx, query, args...)
	if isCardDuplicate(err) {
		return usecase.ErrCardExists
	}
	if err != nil {
		return fmt.Errorf("can't update card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}

	return nil
}

// TouchCard - отмечает, что карту открыли в at. usecase.ErrNotFound, если её нет или она чужая
func (r repository) TouchCard(ctx context.Context, userID uuid.UUID, cardID int64, at time.Time) error {
	stmt := table.Cards.UPDATE(
		table.Cards.LastUsedAt,
	).SET(
		postgres.TimestampzT(at),
	).WHERE(
		table.Cards.ID.EQ(postgres.Int64(cardID)).
			AND(table.Cards.UserID.EQ(postgres.UUID(userID))),
	)

	query, args := stmt.Sql()
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't touch card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}

	return nil
}

// DeleteCard - удаляет карту. usecase.ErrNotFound, если её нет или она чужая
func (r repository) DeleteCard(ctx context.Context, userID uuid.UUID, cardID int64) error {
	stmt := table.Cards.DELETE().WHERE(
		table.Cards.ID.EQ(postgres.Int64(cardID)).
			AND(table.Cards.UserID.EQ(postgres.UUID(userID))),
	)

	query, args := stmt.Sql()
	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("can't delete card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}

	return nil
}
