package qr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"
)

// render - QR-код text картинкой, как его отдаёт Encode
func render(t *testing.T, text string, size int) image.Image {
	t.Helper()

	data, err := Encode(text, size)
	if err != nil {
		t.Fatalf("Encode(%q): %v", text, err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("can't decode png: %v", err)
	}

	return img
}

// screenshot - имитация скриншота приложения магазина: код меньше экрана, вокруг фон и «интерфейс»
func screenshot(code image.Image, bg color.Color) *image.RGBA {
	screen := image.NewRGBA(image.Rect(0, 0, 1080, 2340))
	draw.Draw(screen, screen.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)

	// полосы сверху и снизу: шапка и кнопки
	ui := image.NewUniform(color.RGBA{R: 40, G: 90, B: 200, A: 255})
	draw.Draw(screen, image.Rect(0, 0, 1080, 220), ui, image.Point{}, draw.Src)
	draw.Draw(screen, image.Rect(80, 2000, 1000, 2140), ui, image.Point{}, draw.Src)

	at := image.Pt(240, 800)
	draw.Draw(screen, code.Bounds().Add(at), code, code.Bounds().Min, draw.Src)

	return screen
}

// invert - негатив картинки: светлый код на тёмном фоне
func invert(img image.Image) *image.RGBA {
	b := img.Bounds()
	res := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			res.Set(x, y, color.RGBA{R: 255 - uint8(r>>8), G: 255 - uint8(g>>8), B: 255 - uint8(bl>>8), A: 255})
		}
	}

	return res
}

// rotate90 - картинка, повёрнутая на 90°: фото карты, снятое боком
func rotate90(img image.Image) *image.RGBA {
	b := img.Bounds()
	res := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			res.Set(b.Max.Y-1-y, x-b.Min.X, img.At(x, y))
		}
	}

	return res
}

func TestEncodeDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "card number", text: "2775012345678"},
		{name: "letters and digits", text: "LOYALTY:AB-12345-CD"},
		{name: "url", text: "https://example.com/card?id=42&sig=abc"},
		{name: "cyrillic", text: "Карта №42 — Пятёрочка"},
		{name: "max length", text: strings.Repeat("7", 2000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode(render(t, tt.text, 512))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got != tt.text {
				t.Errorf("Decode = %q, want %q", got, tt.text)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	t.Parallel()

	const text = "9643104512345678"
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	gray := color.RGBA{R: 238, G: 238, B: 238, A: 255}

	tests := []struct {
		name string
		img  func(t *testing.T) image.Image
	}{
		{name: "screenshot", img: func(t *testing.T) image.Image {
			t.Helper()
			return screenshot(render(t, text, 600), white)
		}},
		{name: "screenshot on gray background", img: func(t *testing.T) image.Image {
			t.Helper()
			return screenshot(render(t, text, 600), gray)
		}},
		// тёмная тема в приложении магазина: светлый код на тёмном фоне
		{name: "inverted", img: func(t *testing.T) image.Image {
			t.Helper()
			return invert(screenshot(render(t, text, 600), white))
		}},
		{name: "rotated", img: func(t *testing.T) image.Image {
			t.Helper()
			return rotate90(screenshot(render(t, text, 600), white))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode(tt.img(t))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got != text {
				t.Errorf("Decode = %q, want %q", got, text)
			}
		})
	}
}

func TestDecodeNotFound(t *testing.T) {
	t.Parallel()

	empty := image.NewRGBA(image.Rect(0, 0, 400, 400))
	draw.Draw(empty, empty.Bounds(), image.White, image.Point{}, draw.Src)

	_, err := Decode(screenshot(empty, color.White))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want %v", err, ErrNotFound)
	}
}

func TestEncode(t *testing.T) {
	t.Parallel()

	img := render(t, "2775012345678", 512)
	if b := img.Bounds(); b.Dx() < 512 || b.Dy() < 512 {
		t.Errorf("size = %v, want at least 512×512", b.Size())
	}

	// рамка без единой тёмной точки: сканер на кассе находит код только с ней
	for x := range img.Bounds().Dx() {
		if r, _, _, _ := img.At(x, 0).RGBA(); r != 0xffff {
			t.Fatalf("pixel (%d, 0) is not white: quiet zone is missing", x)
		}
	}

	_, err := Encode("", 512)
	if err == nil {
		t.Error("Encode must reject empty content")
	}
}

// BenchmarkDecodePhoto - фото с камеры телефона: 12 Мп, код занимает малую часть кадра
func BenchmarkDecodePhoto(b *testing.B) {
	data, err := Encode("2775012345678", 600)
	if err != nil {
		b.Fatal(err)
	}
	code, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		b.Fatal(err)
	}

	photo := image.NewRGBA(image.Rect(0, 0, 4000, 3000))
	draw.Draw(photo, photo.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 190, B: 170, A: 255}), image.Point{}, draw.Src)
	draw.Draw(photo, code.Bounds().Add(image.Pt(1700, 1200)), code, code.Bounds().Min, draw.Src)

	for b.Loop() {
		_, err := Decode(photo)
		if err != nil {
			b.Fatal(err)
		}
	}
}
