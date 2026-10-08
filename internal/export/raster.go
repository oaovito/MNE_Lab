package export

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"time"

	"github.com/HugoSmits86/nativewebp"
)

// ImageMeta is the descriptive metadata embedded in image files. It holds
// nothing that identifies a person, a computer or an installation.
type ImageMeta struct {
	Title    string
	Software string
	Created  time.Time
}

// ErrFormat is returned for a format the content cannot be written in.
var ErrFormat = errors.New("export.format_not_compatible")

// EncodeImage writes a rendered figure in a raster format with its
// resolution recorded where the format supports it.
func EncodeImage(im *image.RGBA, format string, dpi float64, background color.Color, meta ImageMeta) ([]byte, error) {
	switch format {
	case "png":
		return encodePNG(im, dpi, meta)
	case "tiff":
		return EncodeTIFF(im, dpi, meta)
	case "jpeg":
		return encodeJPEG(im, dpi, background)
	case "webp":
		var b bytes.Buffer
		if err := nativewebp.Encode(&b, im, nil); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
	return nil, ErrFormat
}

func encodePNG(im *image.RGBA, dpi float64, meta ImageMeta) ([]byte, error) {
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&b, im); err != nil {
		return nil, err
	}
	data := b.Bytes()
	const ihdrEnd = 8 + 4 + 4 + 13 + 4
	if len(data) < ihdrEnd || string(data[12:16]) != "IHDR" {
		return nil, errors.New("export.png_encoding")
	}
	var extra bytes.Buffer
	ppm := uint32(math.Round(dpi / 0.0254))
	phys := make([]byte, 9)
	binary.BigEndian.PutUint32(phys[0:], ppm)
	binary.BigEndian.PutUint32(phys[4:], ppm)
	phys[8] = 1 // unit: meter
	pngChunk(&extra, "pHYs", phys)
	text := func(k, v string) {
		if v != "" {
			pngChunk(&extra, "iTXt", append([]byte(k+"\x00\x00\x00\x00\x00"), []byte(v)...))
		}
	}
	text("Title", meta.Title)
	text("Software", meta.Software)
	if !meta.Created.IsZero() {
		text("Creation Time", meta.Created.UTC().Format(time.RFC3339))
	}
	out := make([]byte, 0, len(data)+extra.Len())
	out = append(out, data[:ihdrEnd]...)
	out = append(out, extra.Bytes()...)
	return append(out, data[ihdrEnd:]...), nil
}

func pngChunk(w *bytes.Buffer, typ string, data []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(data)))
	w.Write(n[:])
	c := crc32.NewIEEE()
	c.Write([]byte(typ))
	c.Write(data)
	w.WriteString(typ)
	w.Write(data)
	binary.BigEndian.PutUint32(n[:], c.Sum32())
	w.Write(n[:])
}

func encodeJPEG(im *image.RGBA, dpi float64, background color.Color) ([]byte, error) {
	if background == nil {
		background = color.White
	}
	// JPEG has no transparency: composite on the chosen background.
	flat := image.NewRGBA(im.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), im, im.Bounds().Min, draw.Over)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, flat, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	data := b.Bytes()
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("export.jpeg_encoding")
	}
	d := uint16(math.Min(math.Round(dpi), 65535))
	app0 := []byte{0xFF, 0xE0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 2, 1, byte(d >> 8), byte(d), byte(d >> 8), byte(d), 0, 0}
	out := make([]byte, 0, len(data)+len(app0))
	out = append(out, data[:2]...)
	out = append(out, app0...)
	return append(out, data[2:]...), nil
}

// EncodeTIFF writes a baseline RGB(A) TIFF, Deflate-compressed with the
// horizontal predictor (lossless), with its resolution in dots per inch.
func EncodeTIFF(im *image.RGBA, dpi float64, meta ImageMeta) ([]byte, error) {
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("export.empty_image")
	}
	opaque := im.Opaque()
	spp := 4
	if opaque {
		spp = 3
	}
	rowBytes := w * spp
	rowsPerStrip := max(1, (256<<10)/rowBytes)
	var strips [][]byte
	row := make([]byte, rowBytes)
	for y0 := 0; y0 < h; y0 += rowsPerStrip {
		var sb bytes.Buffer
		zw, _ := zlib.NewWriterLevel(&sb, zlib.DefaultCompression)
		for y := y0; y < min(h, y0+rowsPerStrip); y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(im.RGBAAt(im.Bounds().Min.X+x, im.Bounds().Min.Y+y)).(color.NRGBA)
				i := x * spp
				row[i], row[i+1], row[i+2] = c.R, c.G, c.B
				if spp == 4 {
					row[i+3] = c.A
				}
			}
			for i := rowBytes - 1; i >= spp; i-- { // horizontal differencing
				row[i] -= row[i-spp]
			}
			zw.Write(row)
		}
		zw.Close()
		strips = append(strips, sb.Bytes())
	}

	type entry struct {
		tag, typ uint16
		count    uint32
		value    []byte // inline when ≤ 4 bytes
	}
	le := binary.LittleEndian
	short := func(v ...uint16) []byte {
		b := make([]byte, 2*len(v))
		for i, x := range v {
			le.PutUint16(b[2*i:], x)
		}
		return b
	}
	long := func(v ...uint32) []byte {
		b := make([]byte, 4*len(v))
		for i, x := range v {
			le.PutUint32(b[4*i:], x)
		}
		return b
	}
	ascii := func(s string) []byte { return append([]byte(s), 0) }
	res := uint32(math.Round(dpi * 100))
	bits := make([]uint16, spp)
	for i := range bits {
		bits[i] = 8
	}
	counts := make([]uint32, len(strips))
	for i, s := range strips {
		counts[i] = uint32(len(s))
	}
	entries := []entry{
		{256, 4, 1, long(uint32(w))},
		{257, 4, 1, long(uint32(h))},
		{258, 3, uint32(spp), short(bits...)},
		{259, 3, 1, short(8)}, // Deflate
		{262, 3, 1, short(2)}, // RGB
	}
	if meta.Title != "" {
		entries = append(entries, entry{270, 2, uint32(len(meta.Title) + 1), ascii(meta.Title)})
	}
	entries = append(entries,
		entry{273, 4, uint32(len(strips)), nil}, // offsets filled below
		entry{277, 3, 1, short(uint16(spp))},
		entry{278, 4, 1, long(uint32(rowsPerStrip))},
		entry{279, 4, uint32(len(strips)), long(counts...)},
		entry{282, 5, 1, long(res, 100)},
		entry{283, 5, 1, long(res, 100)},
		entry{284, 3, 1, short(1)},
		entry{296, 3, 1, short(2)}, // inch
	)
	if meta.Software != "" {
		entries = append(entries, entry{305, 2, uint32(len(meta.Software) + 1), ascii(meta.Software)})
	}
	if !meta.Created.IsZero() {
		entries = append(entries, entry{306, 2, 20, ascii(meta.Created.Format("2006:01:02 15:04:05"))})
	}
	entries = append(entries, entry{317, 3, 1, short(2)}) // horizontal predictor
	if spp == 4 {
		entries = append(entries, entry{338, 3, 1, short(2)}) // unassociated alpha
	}

	// Layout: header, IFD, out-of-line values, strips.
	ifdSize := 2 + 12*len(entries) + 4
	extraOff := 8 + ifdSize
	extra := 0
	for _, e := range entries {
		if e.tag == 273 {
			if len(strips) > 1 {
				extra += 4 * len(strips)
			}
			continue
		}
		if len(e.value) > 4 {
			extra += len(e.value) + len(e.value)%2
		}
	}
	dataOff := extraOff + extra
	offsets := make([]uint32, len(strips))
	pos := uint32(dataOff)
	for i, s := range strips {
		offsets[i] = pos
		pos += uint32(len(s))
	}
	for i := range entries {
		if entries[i].tag == 273 {
			entries[i].value = long(offsets...)
		}
	}
	var out bytes.Buffer
	out.Write([]byte{'I', 'I', 42, 0})
	out.Write(long(8))
	out.Write(short(uint16(len(entries))))
	var blob bytes.Buffer
	for _, e := range entries {
		out.Write(short(e.tag, e.typ))
		out.Write(long(e.count))
		if len(e.value) <= 4 {
			v := make([]byte, 4)
			copy(v, e.value)
			out.Write(v)
			continue
		}
		out.Write(long(uint32(extraOff + blob.Len())))
		blob.Write(e.value)
		if len(e.value)%2 == 1 {
			blob.WriteByte(0)
		}
	}
	out.Write(long(0))
	out.Write(blob.Bytes())
	if out.Len() != dataOff {
		return nil, errors.New("export.tiff_layout")
	}
	for _, s := range strips {
		out.Write(s)
	}
	return out.Bytes(), nil
}
