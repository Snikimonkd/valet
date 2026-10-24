package usecase_test

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
)

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	const token = "token"
	day := 24 * time.Hour
	ttl := 90 * day

	dbErr := errors.New("db is down")
	tests := []struct {
		name string
		// expiresAt - когда истекает сессия в базе
		expiresAt time.Time
		getErr    error
		// wantExtend - сессию продлят до testNow + ttl
		wantExtend bool
		// wantDelete - истёкшую сессию удалят
		wantDelete bool
		wantErr    error
	}{
		// вошли меньше суток назад: продлевать рано
		{name: "fresh", expiresAt: testNow.Add(ttl - time.Hour)},
		{name: "renew after a day", expiresAt: testNow.Add(ttl - day), wantExtend: true},
		{name: "renew after a month", expiresAt: testNow.Add(ttl - 30*day), wantExtend: true},
		{name: "expired", expiresAt: testNow.Add(-time.Minute), wantDelete: true, wantErr: usecase.ErrUnauthorized},
		{name: "expires right now", expiresAt: testNow, wantDelete: true, wantErr: usecase.ErrUnauthorized},
		{name: "unknown token", getErr: usecase.ErrNotFound, wantErr: usecase.ErrUnauthorized},
		{name: "db error", getErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mockRepository(t)
			repo.EXPECT().GetSession(gomock.Any(), token).
				Return(model.Sessions{Token: token, UserID: testUserID, ExpiresAt: tt.expiresAt}, tt.getErr)
			if tt.wantExtend {
				repo.EXPECT().ExtendSession(gomock.Any(), token, testNow.Add(ttl)).Return(nil)
			}
			if tt.wantDelete {
				repo.EXPECT().DeleteSession(gomock.Any(), token).Return(nil)
			}
			u := usecase.NewUsecase(repo, mockClock(testNow))

			session, err := u.Authenticate(t.Context(), token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			want := usecase.Session{Token: token, UserID: testUserID, ExpiresAt: tt.expiresAt}
			if tt.wantExtend {
				want.ExpiresAt, want.Renewed = testNow.Add(ttl), true
			}
			if session != want {
				t.Errorf("session = %+v, want %+v", session, want)
			}
		})
	}
}

func TestAuthenticateEmptyToken(t *testing.T) {
	t.Parallel()

	// пустой токен в базу не ходит
	u := usecase.NewUsecase(mockRepository(t), mockClock(testNow))

	_, err := u.Authenticate(t.Context(), "")
	if !errors.Is(err, usecase.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, usecase.ErrUnauthorized)
	}
}

func TestLogout(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().DeleteSession(gomock.Any(), "token").Return(nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	err := u.Logout(t.Context(), "token")
	if err != nil {
		t.Fatal(err)
	}

	// без куки выходить не из чего: базу не трогаем
	err = u.Logout(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
}
