package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"valet/internal/generated/valet/public/model"
)

// Правила регистрации, которые может нарушить поле. Какое именно поле, говорит ValidationError
var (
	// ErrTooShort - в поле меньше символов, чем нужно
	ErrTooShort = errors.New("too short")
	// ErrBadLogin - в логине есть что-то кроме букв, цифр, точки, дефиса и подчёркивания
	ErrBadLogin = errors.New("bad login")
	// ErrPasswordMismatch - пароль и его повтор не совпали
	ErrPasswordMismatch = errors.New("passwords do not match")
	// ErrLoginTaken - логин уже занят другим пользователем
	ErrLoginTaken = errors.New("login taken")
)

const (
	minLoginLen = 3
	maxLoginLen = 32
	// minPasswordLen - в символах
	minPasswordLen = 8
	// maxPasswordBytes - bcrypt учитывает только первые 72 байта пароля, а длиннее отказывается хешировать
	maxPasswordBytes = 72
	// bcryptCost - около четверти секунды на хеш: подбор пароля по украденной базе выходит долгим
	bcryptCost = 12
)

// RegisterInput - то, что пользователь вводит в форме регистрации
type RegisterInput struct {
	Login           string
	Password        string
	PasswordConfirm string
}

// RegisterRepository - заведение пользователей
type RegisterRepository interface {
	// CreateUser - сохраняет пользователя. ErrLoginTaken, если логин уже занят
	CreateUser(ctx context.Context, user model.Users) error
}

// Register - заводит пользователя и сразу открывает ему сессию. Регистрация открытая: кто угодно, у кого есть
// адрес сайта. ValidationError, если логин или пароль нарушают правила или логин уже занят
func (u usecase) Register(ctx context.Context, in RegisterInput) (Session, error) {
	login := normalizeLogin(in.Login)
	err := validateRegistration(login, in)
	if err != nil {
		return Session{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		return Session{}, fmt.Errorf("can't hash password: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Session{}, fmt.Errorf("can't generate user id: %w", err)
	}

	err = u.repository.CreateUser(ctx, model.Users{
		ID:        id,
		Login:     login,
		Password:  string(hash),
		CreatedAt: u.clock.Now(),
	})
	if errors.Is(err, ErrLoginTaken) {
		return Session{}, ValidationError{FieldLogin: ErrLoginTaken}
	}
	if err != nil {
		return Session{}, err
	}

	return u.openSession(ctx, id)
}

// normalizeLogin - логин без пробелов по краям и в нижнем регистре: «Sasha» и «sasha» - один пользователь
func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

// validateRegistration - правила регистрации: логин из букв, цифр, точки, дефиса и подчёркивания
// длиной от minLoginLen до maxLoginLen, пароль от minPasswordLen символов и не длиннее maxPasswordBytes байт,
// повтор совпадает с паролем
func validateRegistration(login string, in RegisterInput) error {
	invalid := ValidationError{}
	invalid.check(FieldLogin, filled(login), ErrRequired)
	invalid.check(FieldLogin, utf8.RuneCountInString(login) >= minLoginLen, ErrTooShort)
	invalid.check(FieldLogin, fits(login, maxLoginLen), ErrTooLong)
	invalid.check(FieldLogin, loginChars(login), ErrBadLogin)
	invalid.check(FieldPassword, in.Password != "", ErrRequired)
	invalid.check(FieldPassword, utf8.RuneCountInString(in.Password) >= minPasswordLen, ErrTooShort)
	invalid.check(FieldPassword, len(in.Password) <= maxPasswordBytes, ErrTooLong)
	invalid.check(FieldPasswordConfirm, in.PasswordConfirm == in.Password, ErrPasswordMismatch)

	return invalid.err()
}

func loginChars(login string) bool {
	for _, r := range login {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '-' && r != '_' {
			return false
		}
	}

	return true
}
