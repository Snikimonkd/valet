package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	var saved model.Users
	repo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u model.Users) error {
		saved = u
		return nil
	})
	repo.EXPECT().DeleteExpiredSessions(gomock.Any(), gomock.Any(), testNow).Return(nil)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	session, err := u.Register(t.Context(), usecase.RegisterInput{
		Login: " Sasha.K ", Password: "correct horse", PasswordConfirm: "correct horse",
	})
	if err != nil {
		t.Fatal(err)
	}

	// логин хранится в нижнем регистре, id - UUID версии 7 из go, в базе только хеш пароля
	if saved.Login != "sasha.k" || saved.ID.Version() != 7 || !saved.CreatedAt.Equal(testNow) {
		t.Errorf("saved user = %+v", saved)
	}
	if bcrypt.CompareHashAndPassword([]byte(saved.Password), []byte("correct horse")) != nil {
		t.Error("saved password is not a bcrypt hash of the password")
	}
	// после регистрации сразу вошли
	if session.UserID != saved.ID || session.Token == "" {
		t.Errorf("session = %+v, want session of %s", session, saved.ID)
	}
}

func TestRegisterInvalid(t *testing.T) {
	t.Parallel()

	const pw = "password1"
	tests := []struct {
		name  string
		in    usecase.RegisterInput
		field string
		rule  error
	}{
		{name: "empty login", in: usecase.RegisterInput{Login: "  ", Password: pw, PasswordConfirm: pw},
			field: usecase.FieldLogin, rule: usecase.ErrRequired},
		{name: "short login", in: usecase.RegisterInput{Login: "ab", Password: pw, PasswordConfirm: pw},
			field: usecase.FieldLogin, rule: usecase.ErrTooShort},
		{name: "long login", in: usecase.RegisterInput{Login: strings.Repeat("я", 33), Password: pw, PasswordConfirm: pw},
			field: usecase.FieldLogin, rule: usecase.ErrTooLong},
		{name: "space in login", in: usecase.RegisterInput{Login: "sa sha", Password: pw, PasswordConfirm: pw},
			field: usecase.FieldLogin, rule: usecase.ErrBadLogin},
		{name: "at in login", in: usecase.RegisterInput{Login: "me@mail.ru", Password: pw, PasswordConfirm: pw},
			field: usecase.FieldLogin, rule: usecase.ErrBadLogin},
		{name: "empty password", in: usecase.RegisterInput{Login: "sasha"},
			field: usecase.FieldPassword, rule: usecase.ErrRequired},
		{name: "short password", in: usecase.RegisterInput{Login: "sasha", Password: "1234567", PasswordConfirm: "1234567"},
			field: usecase.FieldPassword, rule: usecase.ErrTooShort},
		// bcrypt учитывает только 72 байта: 37 букв кириллицы - это 74
		{name: "password over 72 bytes", in: usecase.RegisterInput{
			Login: "sasha", Password: strings.Repeat("я", 37), PasswordConfirm: strings.Repeat("я", 37),
		}, field: usecase.FieldPassword, rule: usecase.ErrTooLong},
		{name: "mismatch", in: usecase.RegisterInput{Login: "sasha", Password: pw, PasswordConfirm: pw + "!"},
			field: usecase.FieldPasswordConfirm, rule: usecase.ErrPasswordMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// неправильная регистрация до базы не доходит
			u := usecase.NewUsecase(mockRepository(t), mockClock(testNow))

			_, err := u.Register(t.Context(), tt.in)
			invalid, ok := errors.AsType[usecase.ValidationError](err)
			if !ok {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if !errors.Is(invalid[tt.field], tt.rule) {
				t.Errorf("%s: %v, want %v (all: %v)", tt.field, invalid[tt.field], tt.rule, invalid)
			}
		})
	}
}

func TestRegisterCyrillicLogin(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u model.Users) error {
		if u.Login != "саша_1" {
			t.Errorf("login = %q, want саша_1", u.Login)
		}
		return nil
	})
	repo.EXPECT().DeleteExpiredSessions(gomock.Any(), gomock.Any(), testNow).Return(nil)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	in := usecase.RegisterInput{Login: "Саша_1", Password: "password1", PasswordConfirm: "password1"}
	_, err := u.Register(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegisterLoginTaken(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(usecase.ErrLoginTaken)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	in := usecase.RegisterInput{Login: "sasha", Password: "password1", PasswordConfirm: "password1"}
	_, err := u.Register(t.Context(), in)
	invalid, ok := errors.AsType[usecase.ValidationError](err)
	if !ok || !errors.Is(invalid[usecase.FieldLogin], usecase.ErrLoginTaken) {
		t.Errorf("err = %v, want login taken", err)
	}
}

func TestLoginIgnoresCase(t *testing.T) {
	t.Parallel()

	// пользователь зарегистрировался как «sasha», а на телефоне автозамена написала «Sasha»
	repo := mockRepository(t)
	repo.EXPECT().GetUserByLogin(gomock.Any(), "sasha").Return(model.Users{}, usecase.ErrNotFound)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	_, err := u.Login(t.Context(), " Sasha ", "pw")
	if !errors.Is(err, usecase.ErrWrongPassword) {
		t.Errorf("err = %v, want %v", err, usecase.ErrWrongPassword)
	}
}
