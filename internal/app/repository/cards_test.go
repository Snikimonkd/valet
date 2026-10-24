package repository

import (
	"errors"
	"slices"
	"testing"
	"time"

	"valet/internal/app/usecase"
	"valet/internal/pkg/testsupport"
)

func TestCards(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")

	card := testsupport.FakeCard(user.ID)
	id, err := r.CreateCard(t.Context(), card)
	if err != nil {
		t.Fatal(err)
	}
	card.ID = id

	got, err := r.GetCard(t.Context(), user.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != card.Name || got.Content != card.Content || got.Format != "qr" ||
		got.Note != card.Note || !got.CreatedAt.Equal(card.CreatedAt) ||
		!got.LastUsedAt.Equal(card.LastUsedAt) {
		t.Errorf("card = %+v, want %+v", got, card)
	}

	updated := card
	updated.Name, updated.Content, updated.Note = "Лента", "123", ""
	updated.UpdatedAt = card.UpdatedAt.Add(time.Minute)
	err = r.UpdateCard(t.Context(), updated)
	if err != nil {
		t.Fatal(err)
	}
	got, err = r.GetCard(t.Context(), user.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Лента" || got.Content != "123" || got.Note != "" || !got.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Errorf("updated card = %+v", got)
	}
	// правка не трогает дату создания
	if !got.CreatedAt.Equal(card.CreatedAt) {
		t.Errorf("CreatedAt changed: %v -> %v", card.CreatedAt, got.CreatedAt)
	}

	used := testTime(time.Now())
	err = r.TouchCard(t.Context(), user.ID, id, used)
	if err != nil {
		t.Fatal(err)
	}
	got, err = r.GetCard(t.Context(), user.ID, id)
	if err != nil || !got.LastUsedAt.Equal(used) {
		t.Errorf("touched card = %+v, %v", got, err)
	}

	err = r.DeleteCard(t.Context(), user.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.GetCard(t.Context(), user.ID, id)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("deleted card: err = %v, want %v", err, usecase.ErrNotFound)
	}
}

// TestCardsOfAnotherUser - чужую карту не прочитать, не изменить, не открыть и не удалить
func TestCardsOfAnotherUser(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	owner := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	stranger := testsupport.InsertUser(t.Context(), t, r.db, "hash")

	card := testsupport.FakeCard(owner.ID)
	id, err := r.CreateCard(t.Context(), card)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.GetCard(t.Context(), stranger.ID, id)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("GetCard: err = %v, want %v", err, usecase.ErrNotFound)
	}

	hijack := card
	hijack.ID, hijack.UserID, hijack.Name = id, stranger.ID, "чужое"
	err = r.UpdateCard(t.Context(), hijack)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("UpdateCard: err = %v, want %v", err, usecase.ErrNotFound)
	}

	err = r.TouchCard(t.Context(), stranger.ID, id, time.Now())
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("TouchCard: err = %v, want %v", err, usecase.ErrNotFound)
	}

	err = r.DeleteCard(t.Context(), stranger.ID, id)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("DeleteCard: err = %v, want %v", err, usecase.ErrNotFound)
	}

	cards, err := r.ListCards(t.Context(), stranger.ID)
	if err != nil || len(cards) != 0 {
		t.Errorf("stranger sees cards: %+v, %v", cards, err)
	}

	// у владельца карта осталась нетронутой
	got, err := r.GetCard(t.Context(), owner.ID, id)
	if err != nil || got.Name != card.Name ||
		!got.LastUsedAt.Equal(card.LastUsedAt) {
		t.Errorf("owner's card = %+v, %v", got, err)
	}
}

func TestListCardsOrder(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	base := testTime(time.Now())

	// новая карта считается открытой в момент создания
	create := func(name string, created time.Time) int64 {
		card := testsupport.FakeCard(user.ID)
		card.Name, card.CreatedAt, card.UpdatedAt, card.LastUsedAt = name, created, created, created
		id, err := r.CreateCard(t.Context(), card)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	old := create("old", base.Add(-4*time.Hour))
	newer := create("newer", base.Add(-3*time.Hour))
	opened := create("opened", base.Add(-5*time.Hour))
	// создана одновременно с newer: при равном времени выше та, что добавлена позже
	twin := create("twin", base.Add(-3*time.Hour))

	err := r.TouchCard(t.Context(), user.ID, opened, base)
	if err != nil {
		t.Fatal(err)
	}

	cards, err := r.ListCards(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]int64, 0, len(cards))
	for _, c := range cards {
		got = append(got, c.ID)
	}
	// сверху только что открытая, дальше по времени последнего открытия или создания
	want := []int64{opened, twin, newer, old}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestCardsUniqueContent - одну карту дважды не добавить, но у разных пользователей карты могут совпадать
func TestCardsUniqueContent(t *testing.T) {
	t.Parallel()
	r := newTestRepository(t)
	user := testsupport.InsertUser(t.Context(), t, r.db, "hash")
	other := testsupport.InsertUser(t.Context(), t, r.db, "hash")

	card := testsupport.FakeCard(user.ID)
	id, err := r.CreateCard(t.Context(), card)
	if err != nil {
		t.Fatal(err)
	}

	twin := testsupport.FakeCard(user.ID)
	twin.Content = card.Content
	_, err = r.CreateCard(t.Context(), twin)
	if !errors.Is(err, usecase.ErrCardExists) {
		t.Errorf("duplicate: err = %v, want %v", err, usecase.ErrCardExists)
	}

	got, err := r.GetCardByContent(t.Context(), user.ID, card.Content)
	if err != nil || got.ID != id {
		t.Errorf("GetCardByContent = %+v, %v; want card %d", got, err, id)
	}
	_, err = r.GetCardByContent(t.Context(), other.ID, card.Content)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("GetCardByContent of other user: err = %v, want %v", err, usecase.ErrNotFound)
	}

	// та же карта у другого пользователя - не дубль
	foreign := testsupport.FakeCard(other.ID)
	foreign.Content = card.Content
	_, err = r.CreateCard(t.Context(), foreign)
	if err != nil {
		t.Errorf("same content for another user: %v", err)
	}

	second := testsupport.FakeCard(user.ID)
	secondID, err := r.CreateCard(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}

	// правка не может сделать вторую карту копией первой
	second.ID, second.Content = secondID, card.Content
	err = r.UpdateCard(t.Context(), second)
	if !errors.Is(err, usecase.ErrCardExists) {
		t.Errorf("update into duplicate: err = %v, want %v", err, usecase.ErrCardExists)
	}

	// правка карты без смены содержимого - не дубль самой себя
	card.ID, card.Name = id, "новое название"
	err = r.UpdateCard(t.Context(), card)
	if err != nil {
		t.Errorf("update keeping content: %v", err)
	}
}
