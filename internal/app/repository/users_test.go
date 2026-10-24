package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/pkg/testsupport"
)

func TestGetUserByLogin(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")

	got, err := r.GetUserByLogin(t.Context(), user.Login)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || got.Password != "hash" {
		t.Errorf("user = %+v, want %+v", got, user)
	}

	_, err = r.GetUserByLogin(t.Context(), user.Login+"-nope")
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("err = %v, want %v", err, usecase.ErrNotFound)
	}
}

func TestCreateUser(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	user := model.Users{ID: id, Login: "reg-" + id.String(), Password: "hash", CreatedAt: testTime(time.Now())}
	t.Cleanup(func() {
		_, err := r.db.Exec(context.WithoutCancel(t.Context()), "DELETE FROM users WHERE id = $1", id)
		if err != nil {
			t.Errorf("can't delete user: %v", err)
		}
	})

	err = r.CreateUser(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.GetUserByLogin(t.Context(), user.Login)
	if err != nil || got.ID != id || got.Password != "hash" {
		t.Errorf("user = %+v, %v", got, err)
	}

	// второй пользователь с тем же логином
	other, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	err = r.CreateUser(t.Context(), model.Users{ID: other, Login: user.Login, Password: "hash", CreatedAt: user.CreatedAt})
	if !errors.Is(err, usecase.ErrLoginTaken) {
		t.Errorf("err = %v, want %v", err, usecase.ErrLoginTaken)
	}
}
