package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"

	// форматы, которые понимает image.Decode: фото с камеры телефона - JPEG, скриншоты - PNG
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"valet/internal/pkg/qr"
)

var (
	// ErrQRNotFound - на картинке нет QR-кода, который удалось прочитать
	ErrQRNotFound = errors.New("qr code not found")
	// ErrBadImage - файл не картинка или формат не поддерживается
	ErrBadImage = errors.New("bad image")
)

// maxImagePixels - больше точек не распознаём: 50 Мп хватает любой камере телефона,
// а маленький файл с огромными размерами иначе съел бы всю память при декодировании
const maxImagePixels = 50_000_000

// ScanQR - текст QR-кода с фото или скриншота. Картинка нигде не сохраняется.
// ErrBadImage, если это не JPEG, PNG или GIF, ErrQRNotFound, если кода на ней не нашлось
func (u usecase) ScanQR(_ context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("can't read image: %w", err)
	}

	// размеры есть в заголовке файла: проверяем их до того, как выделять память под все точки
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrBadImage
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return "", ErrBadImage
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrBadImage
	}

	text, err := qr.Decode(img)
	if errors.Is(err, qr.ErrNotFound) {
		return "", ErrQRNotFound
	}
	if err != nil {
		return "", err
	}

	return text, nil
}
