package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"valet/internal/generated/valet/public/model"
)

// ErrUnauthorized - сессии с таким токеном нет или она истекла
var ErrUnauthorized = errors.New("unauthorized")

// sessionRenewAfter - как часто продлевать сессию. Продлевать на каждом запросе незачем:
// хватает раза в сутки, а база не получает запись на каждое открытие карты
const sessionRenewAfter = time.Hour * 24

// Session - сессия пользователя: находится по токену и действует до ExpiresAt
type Session struct {
	Token     string
	UserID    uuid.UUID
	ExpiresAt time.Time
	// Renewed - Authenticate только что продлил сессию: куку с токеном тоже пора продлить
	Renewed bool
}

// SessionRepository - хранение сессий
type SessionRepository interface {
	// GetSession - сессия по токену. ErrNotFound, если такой нет
	GetSession(ctx context.Context, token string) (model.Sessions, error)
	// ExtendSession - переносит конец сессии на expiresAt
	ExtendSession(ctx context.Context, token string, expiresAt time.Time) error
	// DeleteSession - удаляет сессию. Удалённой или несуществующей сессии ничего не делает
	DeleteSession(ctx context.Context, token string) error
}

// Authenticate - сессия по токену из куки. Сессия скользящая: раз в sessionRenewAfter заход
// продлевает её на sessionTTL от текущего момента, так что пока карты открывают, вход не слетает.
// ErrUnauthorized, если сессии нет или она истекла
func (u usecase) Authenticate(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrUnauthorized
	}

	stored, err := u.repository.GetSession(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return Session{}, ErrUnauthorized
	}
	if err != nil {
		return Session{}, err
	}

	now := u.clock.Now()
	if !stored.ExpiresAt.After(now) {
		err = u.repository.DeleteSession(ctx, token)
		if err != nil {
			return Session{}, err
		}

		return Session{}, ErrUnauthorized
	}

	session := Session{Token: stored.Token, UserID: stored.UserID, ExpiresAt: stored.ExpiresAt}

	renewed := now.Add(sessionTTL)
	if renewed.Sub(stored.ExpiresAt) >= sessionRenewAfter {
		err = u.repository.ExtendSession(ctx, token, renewed)
		if err != nil {
			return Session{}, err
		}
		session.ExpiresAt, session.Renewed = renewed, true
	}

	return session, nil
}

// Logout - завершает сессию
func (u usecase) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	return u.repository.DeleteSession(ctx, token)
}
