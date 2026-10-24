package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// codeUniqueViolation - код ошибки postgres «нарушен уникальный индекс»
const codeUniqueViolation = "23505"

// isUniqueViolation - база отказалась сохранить строку: она нарушила бы уникальный индекс index
func isUniqueViolation(err error, index string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)

	return ok && pgErr.Code == codeUniqueViolation && pgErr.ConstraintName == index
}

// repository - запросы к postgres: пользователи (users.go), сессии (sessions.go) и карты (cards.go).
// Строки читаются в jet-модели по позиции (pgx.RowToStructByPos): SELECT берёт AllColumns, а jet генерирует
// и колонки, и поля моделей в порядке колонок таблицы, поэтому после make jet они меняются вместе.
// Если всё же разойдутся по числу, pgx вернёт ошибку, а не разложит значения не по тем полям
type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) repository {
	return repository{
		db: db,
	}
}
