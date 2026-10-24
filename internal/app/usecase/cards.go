package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"valet/internal/generated/valet/public/model"
	"valet/internal/pkg/qr"
)

// CardFormat - формат кода на карте. В базе хранится строкой, список допустимых значений только здесь
type CardFormat string

// CardFormatQR - QR-код. Пока единственный формат: штрихкоды добавятся новыми значениями
const CardFormatQR CardFormat = "qr"

// ограничения на поля карты в символах. Содержимое ограничено вместимостью QR-кода в байтах, см. qr.MaxContentLen
const (
	maxNameLen = 100
	maxNoteLen = 500
)

// qrSize - сторона картинки QR-кода в точках: на весь экран телефона без размытия
const qrSize = 720

// ErrCardExists - у пользователя уже есть карта с таким содержимым кода: одну карту дважды не добавить
var ErrCardExists = errors.New("card already exists")

// CardExistsError - карта с таким содержимым уже есть, и вот какая. errors.Is находит в ней ErrCardExists
type CardExistsError struct {
	ID   int64
	Name string
}

func (e CardExistsError) Error() string {
	return fmt.Sprintf("card %d with the same content already exists", e.ID)
}

func (e CardExistsError) Unwrap() error {
	return ErrCardExists
}

// CardInput - то, что пользователь вводит в форме карты
type CardInput struct {
	Name    string
	Content string
	Note    string
}

// CardsRepository - карты пользователей. Каждый запрос ограничен userID: чужую карту не прочитать и не изменить
type CardsRepository interface {
	// ListCards - все карты пользователя: недавно открытые сверху. Новая карта считается открытой при создании
	ListCards(ctx context.Context, userID uuid.UUID) ([]model.Cards, error)
	// GetCard - карта пользователя. ErrNotFound, если её нет или она чужая
	GetCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error)
	// GetCardByContent - карта пользователя с таким содержимым кода. ErrNotFound, если такой нет
	GetCardByContent(ctx context.Context, userID uuid.UUID, content string) (model.Cards, error)
	// CreateCard - сохраняет карту и возвращает её id. ErrCardExists, если карта с таким содержимым уже есть
	CreateCard(ctx context.Context, card model.Cards) (int64, error)
	// UpdateCard - меняет название, содержимое и заметку карты. ErrNotFound, если её нет или она чужая,
	// ErrCardExists, если новое содержимое совпало с другой картой пользователя
	UpdateCard(ctx context.Context, card model.Cards) error
	// TouchCard - отмечает, что карту открыли в at. ErrNotFound, если её нет или она чужая
	TouchCard(ctx context.Context, userID uuid.UUID, cardID int64, at time.Time) error
	// DeleteCard - удаляет карту. ErrNotFound, если её нет или она чужая
	DeleteCard(ctx context.Context, userID uuid.UUID, cardID int64) error
}

// ListCards - все карты пользователя в порядке, удобном на кассе: последние открытые сверху
func (u usecase) ListCards(ctx context.Context, userID uuid.UUID) ([]model.Cards, error) {
	return u.repository.ListCards(ctx, userID)
}

// GetCard - карта пользователя без отметки об открытии: для формы редактирования и офлайн-кэша.
// ErrNotFound, если её нет или она чужая
func (u usecase) GetCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error) {
	return u.repository.GetCard(ctx, userID, cardID)
}

// OpenCard - карту показывают на кассе: отдаёт её и поднимает наверх списка.
// ErrNotFound, если её нет или она чужая
func (u usecase) OpenCard(ctx context.Context, userID uuid.UUID, cardID int64) (model.Cards, error) {
	card, err := u.repository.GetCard(ctx, userID, cardID)
	if err != nil {
		return model.Cards{}, err
	}

	now := u.clock.Now()
	err = u.repository.TouchCard(ctx, userID, cardID, now)
	if err != nil {
		return model.Cards{}, err
	}
	card.LastUsedAt = now

	return card, nil
}

// CreateCard - сохраняет новую QR-карту пользователя и возвращает её id.
// ValidationError, если карта нарушает правила, CardExistsError, если такая карта уже есть
func (u usecase) CreateCard(ctx context.Context, userID uuid.UUID, in CardInput) (int64, error) {
	in = normalizeCard(in)
	err := validateCard(in)
	if err != nil {
		return 0, err
	}

	now := u.clock.Now()

	id, err := u.repository.CreateCard(ctx, model.Cards{
		UserID:    userID,
		Name:      in.Name,
		Content:   in.Content,
		Format:    string(CardFormatQR),
		Note:      in.Note,
		CreatedAt: now,
		UpdatedAt: now,
		// новая карта встаёт наверх списка, как только что открытая
		LastUsedAt: now,
	})
	if errors.Is(err, ErrCardExists) {
		return 0, u.cardExists(ctx, userID, in.Content)
	}

	return id, err
}

// UpdateCard - меняет карту пользователя. ValidationError, если карта нарушает правила,
// ErrNotFound, если её нет или она чужая, CardExistsError, если новое содержимое - как у другой карты
func (u usecase) UpdateCard(ctx context.Context, userID uuid.UUID, cardID int64, in CardInput) error {
	in = normalizeCard(in)
	err := validateCard(in)
	if err != nil {
		return err
	}

	err = u.repository.UpdateCard(ctx, model.Cards{
		ID:        cardID,
		UserID:    userID,
		Name:      in.Name,
		Content:   in.Content,
		Note:      in.Note,
		UpdatedAt: u.clock.Now(),
	})
	if errors.Is(err, ErrCardExists) {
		return u.cardExists(ctx, userID, in.Content)
	}

	return err
}

// cardExists - база отказалась сохранить дубль: находит карту, которая уже есть, чтобы показать, какая.
// Если её успели удалить между запросами, отдаёт просто ErrCardExists
func (u usecase) cardExists(ctx context.Context, userID uuid.UUID, content string) error {
	card, err := u.repository.GetCardByContent(ctx, userID, content)
	if errors.Is(err, ErrNotFound) {
		return ErrCardExists
	}
	if err != nil {
		return err
	}

	return CardExistsError{ID: card.ID, Name: card.Name}
}

// DeleteCard - удаляет карту пользователя. ErrNotFound, если её нет или она чужая
func (u usecase) DeleteCard(ctx context.Context, userID uuid.UUID, cardID int64) error {
	return u.repository.DeleteCard(ctx, userID, cardID)
}

// CardQR - PNG с QR-кодом карты пользователя. ErrNotFound, если её нет или она чужая
func (u usecase) CardQR(ctx context.Context, userID uuid.UUID, cardID int64) ([]byte, error) {
	card, err := u.repository.GetCard(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}

	return qr.Encode(card.Content, qrSize)
}

// normalizeCard - убирает пробелы по краям. В содержимом тоже: QR-коды карт с пробелом по краю
// не встречаются, а при ручном вводе он почти всегда случайный
func normalizeCard(in CardInput) CardInput {
	return CardInput{
		Name:    strings.TrimSpace(in.Name),
		Content: strings.TrimSpace(in.Content),
		Note:    strings.TrimSpace(in.Note),
	}
}

// validateCard - правила карты: название и содержимое заполнены, ничего не длиннее своего предела,
// содержимое влезает в один QR-код
func validateCard(in CardInput) error {
	invalid := ValidationError{}
	invalid.check(FieldName, filled(in.Name), ErrRequired)
	invalid.check(FieldName, fits(in.Name, maxNameLen), ErrTooLong)
	invalid.check(FieldContent, filled(in.Content), ErrRequired)
	invalid.check(FieldContent, len(in.Content) <= qr.MaxContentLen, ErrTooLong)
	invalid.check(FieldNote, fits(in.Note, maxNoteLen), ErrTooLong)

	return invalid.err()
}
