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
	// ErrQRNotFound - на картинке нет QR-кода, который удалось прочитать. Это та же ошибка, что отдаёт qr.Decode,
	// поэтому её не надо оборачивать: причина от gozxing уже внутри
	ErrQRNotFound = qr.ErrNotFound
	// ErrBadImage - файл не картинка или формат не поддерживается
	ErrBadImage = errors.New("bad image")
)

// maxImagePixels - больше точек не распознаём: 50 Мп хватает любой камере телефона,
// а маленький файл с огромными размерами иначе съел бы всю память при декодировании
const maxImagePixels = 50_000_000

// ScanQR - текст QR-кода с фото или скриншота. Картинка нигде не сохраняется.
// ErrBadImage, если это не JPEG, PNG или GIF, ErrQRNotFound, если кода на ней не нашлось.
// Обе ошибки несут причину: что именно не так с файлом, она нужна в логах
func (u usecase) ScanQR(_ context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("can't read image: %w", err)
	}

	// размеры есть в заголовке файла: проверяем их до того, как выделять память под все точки
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: can't read image header: %w", ErrBadImage, err)
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return "", fmt.Errorf("%w: %dx%d is more than %d pixels", ErrBadImage, cfg.Width, cfg.Height, maxImagePixels)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: can't decode image: %w", ErrBadImage, err)
	}

	return qr.Decode(img)
}
