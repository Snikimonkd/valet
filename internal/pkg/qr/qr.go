// Package qr - распознавание QR-кодов на картинках и рендер QR-кодов в PNG. Обёртка над gozxing:
// остальной код о библиотеке не знает, её можно заменить, не трогая usecase
package qr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"unicode/utf8"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// ErrNotFound - на картинке нет QR-кода, который удалось прочитать
var ErrNotFound = errors.New("qr code not found")

// MaxContentLen - сколько байт текста влезает в один QR-код с уровнем коррекции M
const MaxContentLen = 2331

// quietZone - белая рамка вокруг кода в модулях. Стандарт требует 4, без неё сканеры промахиваются
const quietZone = 4

// Decode - текст первого QR-кода на картинке. Сначала читает как есть, потом в инверсии:
// светлый код на тёмном фоне (тёмная тема в приложении магазина) без инверсии не находится.
// ErrNotFound, если кода нет или его не прочитать
func Decode(img image.Image) (string, error) {
	src := gozxing.NewLuminanceSourceFromImage(img)

	text, err := decode(src)
	if errors.Is(err, ErrNotFound) {
		text, err = decode(gozxing.NewInvertedLuminanceSource(src))
	}

	return text, err
}

func decode(src gozxing.LuminanceSource) (string, error) {
	bmp, err := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(src))
	if err != nil {
		return "", fmt.Errorf("can't binarize image: %w", err)
	}

	// TRY_HARDER - дольше ищет код на картинке, где кроме него много всего: скриншот целого экрана
	hints := map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_TRY_HARDER: true}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, hints)
	if err != nil {
		// любая ошибка чтения значит одно: код не прочитан. Подробности gozxing наружу не нужны
		return "", ErrNotFound
	}

	return res.GetText(), nil
}

// Encode - PNG с QR-кодом text размером не меньше size×size точек: чёрный код на белом с рамкой quietZone.
// Коррекция M: код читается, даже если экран в бликах или поцарапан
func Encode(text string, size int) ([]byte, error) {
	if text == "" {
		return nil, errors.New("empty qr content")
	}

	hints := map[gozxing.EncodeHintType]any{
		gozxing.EncodeHintType_ERROR_CORRECTION: "M",
		gozxing.EncodeHintType_MARGIN:           quietZone,
	}
	// ASCII кодируется как есть, без пометки о кодировке: номер карты в коде совпадёт байт в байт
	// с исходным. Остальное - в UTF-8 с пометкой ECI, иначе кириллица превратится в знаки вопроса
	if !isASCII(text) {
		hints[gozxing.EncodeHintType_CHARACTER_SET] = "UTF-8"
	}

	matrix, err := qrcode.NewQRCodeWriter().Encode(text, gozxing.BarcodeFormat_QR_CODE, size, size, hints)
	if err != nil {
		return nil, fmt.Errorf("can't encode qr: %w", err)
	}

	var buf bytes.Buffer
	err = png.Encode(&buf, matrix)
	if err != nil {
		return nil, fmt.Errorf("can't encode png: %w", err)
	}

	return buf.Bytes(), nil
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}
