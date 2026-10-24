package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"valet/internal/generated/valet/public/model"
)

// ErrWrongPassword - логин или пароль неверный. Снаружи эти случаи неотличимы
var ErrWrongPassword = errors.New("wrong password")

// sessionTTL - сколько живёт сессия без заходов. Каждый заход продлевает её на столько же, см. Authenticate
const sessionTTL = time.Hour * 24 * 90

// LoginRepository - пользователи и открытие сессий
type LoginRepository interface {
	// GetUserByLogin - пользователь с bcrypt-хешем пароля. ErrNotFound, если такого логина нет
	GetUserByLogin(ctx context.Context, login string) (model.Users, error)
	// CreateSession - сохраняет новую сессию
	CreateSession(ctx context.Context, session model.Sessions) error
	// DeleteExpiredSessions - удаляет сессии пользователя, истёкшие к now
	DeleteExpiredSessions(ctx context.Context, userID uuid.UUID, now time.Time) error
}

// Login - проверяет логин и пароль и открывает сессию. Логин без учёта регистра, см. normalizeLogin.
// ErrWrongPassword, если такого логина нет или пароль неверный
func (u usecase) Login(ctx context.Context, login, password string) (Session, error) {
	user, err := u.repository.GetUserByLogin(ctx, normalizeLogin(login))
	if errors.Is(err, ErrNotFound) {
		return Session{}, ErrWrongPassword
	}
	if err != nil {
		return Session{}, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return Session{}, ErrWrongPassword
	}

	return u.openSession(ctx, user.ID)
}

// openSession - новая сессия пользователя: после входа и после регистрации
func (u usecase) openSession(ctx context.Context, userID uuid.UUID) (Session, error) {
	now := u.clock.Now()
	// сессии копятся с каждого телефона и браузера: при входе убираем протухшие, чтобы таблица не росла вечно
	err := u.repository.DeleteExpiredSessions(ctx, userID, now)
	if err != nil {
		return Session{}, err
	}

	session := Session{
		// 128 случайных бит: токен не подобрать
		Token:     rand.Text(),
		UserID:    userID,
		ExpiresAt: now.Add(sessionTTL),
	}
	err = u.repository.CreateSession(ctx, model.Sessions{
		Token:     session.Token,
		UserID:    session.UserID,
		CreatedAt: now,
		ExpiresAt: session.ExpiresAt,
	})
	if err != nil {
		return Session{}, err
	}

	return session, nil
}
