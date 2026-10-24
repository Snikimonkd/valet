package usecase_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"valet/internal/app/usecase"
	"valet/internal/generated/valet/public/model"
	"valet/internal/pkg/qr"
)

func TestCreateCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   usecase.CardInput
		// want - что уйдёт в базу, если карта правильная
		want    model.Cards
		wantErr map[string]error
	}{
		{
			name: "ok",
			in:   usecase.CardInput{Name: "Пятёрочка", Content: "2775012345678", Note: "PIN 1234"},
			want: model.Cards{Name: "Пятёрочка", Content: "2775012345678", Note: "PIN 1234"},
		},
		{
			name: "trims spaces",
			in:   usecase.CardInput{Name: "  Лента ", Content: " 123\n", Note: " "},
			want: model.Cards{Name: "Лента", Content: "123"},
		},
		{
			name:    "empty",
			in:      usecase.CardInput{Name: " ", Content: ""},
			wantErr: map[string]error{usecase.FieldName: usecase.ErrRequired, usecase.FieldContent: usecase.ErrRequired},
		},
		{
			name: "too long",
			in: usecase.CardInput{
				Name:    strings.Repeat("я", 101),
				Content: strings.Repeat("7", qr.MaxContentLen+1),
				Note:    strings.Repeat("я", 501),
			},
			wantErr: map[string]error{
				usecase.FieldName:    usecase.ErrTooLong,
				usecase.FieldContent: usecase.ErrTooLong,
				usecase.FieldNote:    usecase.ErrTooLong,
			},
		},
		// пределы в символах, а не в байтах: кириллица в названии не съедает лимит вдвое
		{
			name: "limits are in runes",
			in:   usecase.CardInput{Name: strings.Repeat("я", 100), Content: "1", Note: strings.Repeat("я", 500)},
			want: model.Cards{Name: strings.Repeat("я", 100), Content: "1", Note: strings.Repeat("я", 500)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mockRepository(t)
			if tt.wantErr == nil {
				want := tt.want
				want.UserID = testUserID
				want.Format = string(usecase.CardFormatQR)
				want.CreatedAt, want.UpdatedAt, want.LastUsedAt = testNow, testNow, testNow
				repo.EXPECT().CreateCard(gomock.Any(), want).Return(int64(42), nil)
			}
			u := usecase.NewUsecase(repo, mockClock(testNow))

			id, err := u.CreateCard(t.Context(), testUserID, tt.in)
			if tt.wantErr == nil {
				if err != nil || id != 42 {
					t.Fatalf("CreateCard = %d, %v", id, err)
				}
				return
			}

			var invalid usecase.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if len(invalid) != len(tt.wantErr) {
				t.Errorf("invalid = %v, want %v", invalid, tt.wantErr)
			}
			for field, rule := range tt.wantErr {
				if !errors.Is(invalid[field], rule) {
					t.Errorf("%s: %v, want %v", field, invalid[field], rule)
				}
			}
		})
	}
}

func TestUpdateCard(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().UpdateCard(gomock.Any(), model.Cards{
		ID: 3, UserID: testUserID, Name: "Лента", Content: "123", Note: "", UpdatedAt: testNow,
	}).Return(nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	err := u.UpdateCard(t.Context(), testUserID, 3, usecase.CardInput{Name: " Лента", Content: "123 "})
	if err != nil {
		t.Fatal(err)
	}

	// неправильная карта до базы не доходит
	err = u.UpdateCard(t.Context(), testUserID, 3, usecase.CardInput{})
	if _, ok := errors.AsType[usecase.ValidationError](err); !ok {
		t.Errorf("err = %v, want ValidationError", err)
	}
}

func TestOpenCard(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().GetCard(gomock.Any(), testUserID, int64(3)).Return(model.Cards{ID: 3, UserID: testUserID}, nil)
	repo.EXPECT().TouchCard(gomock.Any(), testUserID, int64(3), testNow).Return(nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	card, err := u.OpenCard(t.Context(), testUserID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !card.LastUsedAt.Equal(testNow) {
		t.Errorf("LastUsedAt = %v, want %v", card.LastUsedAt, testNow)
	}
}

func TestOpenCardNotFound(t *testing.T) {
	t.Parallel()

	// чужую карту не открыть и не поднять наверх чужого списка
	repo := mockRepository(t)
	repo.EXPECT().GetCard(gomock.Any(), testUserID, int64(3)).Return(model.Cards{}, usecase.ErrNotFound)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	_, err := u.OpenCard(t.Context(), testUserID, 3)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("err = %v, want %v", err, usecase.ErrNotFound)
	}
}

func TestCardQR(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().GetCard(gomock.Any(), testUserID, int64(3)).Return(model.Cards{Content: "2775012345678"}, nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	data, err := u.CardQR(t.Context(), testUserID, 3)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// на картинке ровно то, что в карте: иначе касса прочитает другой номер
	got, err := qr.Decode(img)
	if err != nil || got != "2775012345678" {
		t.Errorf("qr content = %q, %v", got, err)
	}
}

func TestScanQR(t *testing.T) {
	t.Parallel()

	code, err := qr.Encode("2775012345678", 400)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr error
	}{
		{name: "qr", data: code, want: "2775012345678"},
		{name: "no qr", data: blankPNG(t), wantErr: usecase.ErrQRNotFound},
		{name: "not an image", data: []byte("hello"), wantErr: usecase.ErrBadImage},
		// заголовок PNG обещает 100 000 × 100 000 точек: распаковывать такое нельзя
		{name: "too many pixels", data: pngHeader(100_000, 100_000), wantErr: usecase.ErrBadImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := usecase.NewUsecase(mockRepository(t), mockClock(testNow))

			got, err := u.ScanQR(t.Context(), bytes.NewReader(tt.data))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ScanQR = %q, want %q", got, tt.want)
			}
		})
	}
}

// blankPNG - белый лист 400×400: картинка настоящая, а кода на ней нет
func blankPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewGray(image.Rect(0, 0, 400, 400))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// pngHeader - начало PNG-файла, где заголовок обещает картинку width × height. Самих точек в файле нет
func pngHeader(width, height uint32) []byte {
	ihdr := make([]byte, 0, 17)
	ihdr = append(ihdr, "IHDR"...)
	ihdr = binary.BigEndian.AppendUint32(ihdr, width)
	ihdr = binary.BigEndian.AppendUint32(ihdr, height)
	// 8 бит на канал, RGB, без хитростей
	ihdr = append(ihdr, 8, 2, 0, 0, 0)

	res := []byte("\x89PNG\r\n\x1a\n")
	res = binary.BigEndian.AppendUint32(res, uint32(len(ihdr)-4))
	res = append(res, ihdr...)

	return binary.BigEndian.AppendUint32(res, crc32.ChecksumIEEE(ihdr))
}

func TestCreateCardExists(t *testing.T) {
	t.Parallel()

	in := usecase.CardInput{Name: "Лента", Content: "123"}
	tests := []struct {
		name string
		// found - что найдётся по содержимому после отказа базы
		found    model.Cards
		foundErr error
		want     error
	}{
		{
			name:  "shows which card",
			found: model.Cards{ID: 3, Name: "Пятёрочка"},
			want:  usecase.CardExistsError{ID: 3, Name: "Пятёрочка"},
		},
		// карту удалили между вставкой и поиском: дубль был, но показать нечего
		{name: "deleted meanwhile", foundErr: usecase.ErrNotFound, want: usecase.ErrCardExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := mockRepository(t)
			repo.EXPECT().CreateCard(gomock.Any(), gomock.Any()).Return(int64(0), usecase.ErrCardExists)
			repo.EXPECT().GetCardByContent(gomock.Any(), testUserID, "123").Return(tt.found, tt.foundErr)
			u := usecase.NewUsecase(repo, mockClock(testNow))

			_, err := u.CreateCard(t.Context(), testUserID, in)
			if err != tt.want { //nolint:errorlint // проверяем саму ошибку: в ней должна быть найденная карта
				t.Errorf("err = %#v, want %#v", err, tt.want)
			}
			if !errors.Is(err, usecase.ErrCardExists) {
				t.Errorf("errors.Is(%v, ErrCardExists) = false", err)
			}
		})
	}
}

func TestUpdateCardExists(t *testing.T) {
	t.Parallel()

	repo := mockRepository(t)
	repo.EXPECT().UpdateCard(gomock.Any(), gomock.Any()).Return(usecase.ErrCardExists)
	repo.EXPECT().GetCardByContent(gomock.Any(), testUserID, "123").Return(model.Cards{ID: 3, Name: "Пятёрочка"}, nil)
	u := usecase.NewUsecase(repo, mockClock(testNow))

	err := u.UpdateCard(t.Context(), testUserID, 5, usecase.CardInput{Name: "Лента", Content: "123"})
	exists, ok := errors.AsType[usecase.CardExistsError](err)
	if !ok || exists.ID != 3 {
		t.Errorf("err = %v, want CardExistsError for card 3", err)
	}
}
