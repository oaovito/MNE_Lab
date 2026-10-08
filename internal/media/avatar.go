// Package media validates and normalizes user images (profile avatars).
package media

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP decoding
)

// MaxAvatarBytes is the documented upload limit.
const MaxAvatarBytes = 5 << 20

// Avatar errors are stable identifiers the interface localizes.
var (
	ErrTooLarge      = errors.New("avatar.too_large")
	ErrUnsupported   = errors.New("avatar.unsupported_format")
	ErrInvalidImage  = errors.New("avatar.invalid_image")
	ErrTooSmall      = errors.New("avatar.too_small")
	ErrTooManyPixels = errors.New("avatar.too_many_pixels")
)

const maxSide = 1024

// Sniff identifies the format from content, never from the file name.
func Sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// NormalizeAvatar validates an uploaded image and returns bytes safe to
// store. Still images are decoded and re-encoded, which strips embedded
// metadata such as camera details and GPS location. GIFs are fully decoded
// to validate every frame and then kept as uploaded so animation survives.
func NormalizeAvatar(b []byte) (out []byte, mime string, err error) {
	if len(b) > MaxAvatarBytes {
		return nil, "", ErrTooLarge
	}
	mime = Sniff(b)
	if mime == "" {
		return nil, "", ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, "", ErrInvalidImage
	}
	if cfg.Width < 16 || cfg.Height < 16 {
		return nil, "", ErrTooSmall
	}
	if cfg.Width*cfg.Height > 40_000_000 {
		return nil, "", ErrTooManyPixels
	}
	if mime == "image/gif" {
		g, err := gif.DecodeAll(bytes.NewReader(b))
		if err != nil || len(g.Image) == 0 {
			return nil, "", ErrInvalidImage
		}
		if len(g.Image) > 1000 {
			return nil, "", ErrInvalidImage
		}
		return b, mime, nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", ErrInvalidImage
	}
	img = fit(img, maxSide)
	var buf bytes.Buffer
	if mime == "image/jpeg" {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	} else {
		// PNG and WebP are stored as PNG (lossless, universally decodable).
		mime = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	if err != nil {
		return nil, "", ErrInvalidImage
	}
	return buf.Bytes(), mime, nil
}

func fit(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	if w >= h {
		h = h * max / w
		w = max
	} else {
		w = w * max / h
		h = max
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
