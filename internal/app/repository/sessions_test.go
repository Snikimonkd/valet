package repository

import (
	"errors"
	"testing"
	"time"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/pkg/testsupport"
)

func TestSessions(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	now := testTime(time.Now())

	session := model.Sessions{Token: "tok-" + user.Login, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	err := r.CreateSession(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.GetSession(t.Context(), session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != user.ID || !got.ExpiresAt.Equal(session.ExpiresAt) {
		t.Errorf("session = %+v, want %+v", got, session)
	}

	later := now.Add(48 * time.Hour)
	err = r.ExtendSession(t.Context(), session.Token, later)
	if err != nil {
		t.Fatal(err)
	}
	got, err = r.GetSession(t.Context(), session.Token)
	if err != nil || !got.ExpiresAt.Equal(later) {
		t.Errorf("extended session = %+v, %v; want expiry %v", got, err, later)
	}

	err = r.DeleteSession(t.Context(), session.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.GetSession(t.Context(), session.Token)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("deleted session: err = %v, want %v", err, usecase.ErrNotFound)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	other := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	now := testTime(time.Now())

	sessions := []model.Sessions{
		{Token: "expired-" + user.Login, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
		{Token: "alive-" + user.Login, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		// истёкшая сессия другого пользователя: её удалит только его собственный вход
		{Token: "expired-" + other.Login, UserID: other.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
	}
	for _, s := range sessions {
		err := r.CreateSession(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
	}

	err := r.DeleteExpiredSessions(t.Context(), user.ID, now)
	if err != nil {
		t.Fatal(err)
	}

	for i, wantAlive := range []bool{false, true, true} {
		_, err := r.GetSession(t.Context(), sessions[i].Token)
		if alive := err == nil; alive != wantAlive {
			t.Errorf("session %s: alive = %v, want %v (err %v)", sessions[i].Token, alive, wantAlive, err)
		}
	}
}
