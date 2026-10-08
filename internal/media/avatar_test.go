package media

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func solid(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{30, 90, 160, 255})
		}
	}
	return img
}

func TestFormats(t *testing.T) {
	var p, j bytes.Buffer
	png.Encode(&p, solid(2000, 1000))
	jpeg.Encode(&j, solid(64, 64), nil)
	out, mime, err := NormalizeAvatar(p.Bytes())
	if err != nil || mime != "image/png" {
		t.Fatalf("png: %v %s", err, mime)
	}
	cfg, _, _ := image.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 1024 || cfg.Height != 512 {
		t.Fatalf("not resized: %dx%d", cfg.Width, cfg.Height)
	}
	if _, mime, err := NormalizeAvatar(j.Bytes()); err != nil || mime != "image/jpeg" {
		t.Fatalf("jpeg: %v", err)
	}
}

func TestAnimatedGIFPreserved(t *testing.T) {
	g := &gif.GIF{}
	for i := 0; i < 3; i++ {
		fr := image.NewPaletted(image.Rect(0, 0, 32, 32), palette.Plan9)
		fr.SetColorIndex(i, i, uint8(i*40))
		g.Image = append(g.Image, fr)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	gif.EncodeAll(&b, g)
	out, mime, err := NormalizeAvatar(b.Bytes())
	if err != nil || mime != "image/gif" || !bytes.Equal(out, b.Bytes()) {
		t.Fatalf("gif not preserved: %v %s", err, mime)
	}
	back, _ := gif.DecodeAll(bytes.NewReader(out))
	if len(back.Image) != 3 {
		t.Fatal("frames lost")
	}
}

func TestRejections(t *testing.T) {
	if _, _, err := NormalizeAvatar(make([]byte, MaxAvatarBytes+1)); err != ErrTooLarge {
		t.Fatalf("size: %v", err)
	}
	if _, _, err := NormalizeAvatar([]byte("%PDF-1.7 not an image")); err != ErrUnsupported {
		t.Fatalf("format: %v", err)
	}
	fake := append([]byte("\x89PNG\r\n\x1a\n"), []byte("garbage")...)
	if _, _, err := NormalizeAvatar(fake); err != ErrInvalidImage {
		t.Fatalf("corrupt: %v", err)
	}
	var tiny bytes.Buffer
	png.Encode(&tiny, solid(4, 4))
	if _, _, err := NormalizeAvatar(tiny.Bytes()); err != ErrTooSmall {
		t.Fatalf("tiny: %v", err)
	}
}
