package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

func TestLogin(t *testing.T) {
	t.Parallel()

	const password = "secret"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := model.Users{ID: testUserID, Login: "me", Password: string(hash)}

	dbErr := errors.New("db is down")
	tests := []struct {
		name     string
		password string
		repoErr  error
		wantErr  error
	}{
		{name: "ok", password: password},
		{name: "wrong password", password: "nope", wantErr: usecase.ErrWrongPassword},
		// неизвестный логин снаружи неотличим от неверного пароля
		{name: "unknown login", password: password, repoErr: usecase.ErrNotFound, wantErr: usecase.ErrWrongPassword},
		{name: "db error", password: password, repoErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mockRepository(t)
			repo.EXPECT().GetUserByLogin(gomock.Any(), "me").Return(user, tt.repoErr)

			var saved model.Sessions
			if tt.wantErr == nil {
				repo.EXPECT().DeleteExpiredSessions(gomock.Any(), user.ID, testNow).Return(nil)
				repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, s model.Sessions) error {
						saved = s
						return nil
					},
				)
			}
			u := usecase.NewUsecase(repo, mockClock(testNow))

			session, err := u.Login(t.Context(), "me", tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if session != (usecase.Session{}) {
					t.Errorf("failed login must not open a session: %+v", session)
				}
				return
			}

			// сессия на 90 дней по часам usecase, в базу уходит она же
			wantExpiry := testNow.Add(90 * 24 * time.Hour)
			if session.Token == "" || session.UserID != user.ID || !session.ExpiresAt.Equal(wantExpiry) {
				t.Errorf("session = %+v", session)
			}
			if saved.Token != session.Token || saved.UserID != user.ID ||
				!saved.CreatedAt.Equal(testNow) || !saved.ExpiresAt.Equal(wantExpiry) {
				t.Errorf("saved session = %+v", saved)
			}
		})
	}
}

func TestLoginTokensAreUnique(t *testing.T) {
	t.Parallel()

	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	repo := mockRepository(t)
	user := model.Users{ID: testUserID, Password: string(hash)}
	repo.EXPECT().GetUserByLogin(gomock.Any(), "me").Return(user, nil).Times(2)
	repo.EXPECT().DeleteExpiredSessions(gomock.Any(), testUserID, testNow).Return(nil).Times(2)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	first, err := u.Login(t.Context(), "me", "pw")
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.Login(t.Context(), "me", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Error("two logins got the same token")
	}
}
