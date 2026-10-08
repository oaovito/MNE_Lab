package export

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func tr(key string, kv ...string) string {
	s := map[string]string{"point.weeks": "Week {n}", "point.days": "Day {n}", "col.sample_id": "Sample ID", "col.diameter": "Diameter",
		"param.effective_diameter": "Effective Diameter", "sheet.data": "Data"}[key]
	if s == "" {
		s = key
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

func testImage(transparent bool) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, 301, 157))
	for y := 0; y < 157; y++ {
		for x := 0; x < 301; x++ {
			a := uint8(255)
			if transparent && x < 50 {
				a = uint8(x * 5)
			}
			c := color.NRGBA{uint8(x), uint8(y), uint8(x ^ y), a}
			im.Set(x, y, c)
		}
	}
	return im
}

func TestFormatsAndRecommendations(t *testing.T) {
	g := For(KindGraph)
	if strings.Join(g.Sections[0].Formats, ",") != "png,svg,pdf,tiff" {
		t.Fatalf("graph recommended: %v", g.Sections[0].Formats)
	}
	if !Compatible(KindGraph, "jpeg") || !Compatible(KindGraph, "csv") || Compatible(KindGraph, "original") {
		t.Fatal("graph compatibility")
	}
	if Compatible(KindDataset, "png") {
		t.Fatal("a dataset must not be offered as an image")
	}
	if SmartDefault(KindGraph, PresetPublication).Formats[0] != "svg" || SmartDefault(KindGraph, PresetPresentation).Formats[0] != "png" ||
		SmartDefault(KindCycle, "").Formats[0] != "package" || SmartDefault(KindDataset, "").Formats[0] != "xlsx" {
		t.Fatal("smart defaults")
	}
	for id, f := range Formats {
		if f.ID != id {
			t.Fatalf("format id %s", id)
		}
	}
	if Formats["jpeg"].Warning == "" || !Formats["jpeg"].Lossy {
		t.Fatal("jpeg must warn about lossy compression")
	}
}

func TestRasterFormatsKeepPixelsAndDPI(t *testing.T) {
	im := testImage(true)
	meta := ImageMeta{Title: "A1 DLS", Software: "MNE Lab test", Created: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}

	b, err := EncodeImage(im, "png", 600, nil, meta)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := png.Decode(bytes.NewReader(b))
	if err != nil || !samePixels(im, dec) {
		t.Fatalf("png: %v", err)
	}
	i := bytes.Index(b, []byte("pHYs"))
	if i < 0 || binary.BigEndian.Uint32(b[i+4:]) != 23622 || b[i+12] != 1 {
		t.Fatal("png pHYs missing or wrong")
	}

	b, err = EncodeImage(im, "tiff", 600, nil, meta)
	if err != nil {
		t.Fatal(err)
	}
	dec, err = tiff.Decode(bytes.NewReader(b))
	if err != nil || !samePixels(im, dec) {
		t.Fatalf("tiff: %v", err)
	}
	opaque := testImage(false)
	b, _ = EncodeImage(opaque, "tiff", 300, nil, meta)
	if dec, err = tiff.Decode(bytes.NewReader(b)); err != nil || !samePixels(opaque, dec) {
		t.Fatalf("opaque tiff: %v", err)
	}
	if os.Getenv("MNELAB_EXPORT_OUT") != "" {
		os.WriteFile(filepath.Join(os.Getenv("MNELAB_EXPORT_OUT"), "test.tiff"), b, 0o644)
	}

	b, err = EncodeImage(im, "jpeg", 300, color.White, meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	if string(b[6:11]) != "JFIF\x00" || b[13] != 1 || binary.BigEndian.Uint16(b[14:]) != 300 {
		t.Fatal("jpeg density")
	}

	b, err = EncodeImage(im, "webp", 300, nil, meta)
	if err != nil {
		t.Fatal(err)
	}
	if dec, err = webp.Decode(bytes.NewReader(b)); err != nil || !samePixels(im, dec) {
		t.Fatalf("webp must be lossless: %v", err)
	}
	if _, err := EncodeImage(im, "csv", 300, nil, meta); err != ErrFormat {
		t.Fatal("csv is not an image format")
	}
}

func samePixels(a *image.RGBA, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			ca := color.NRGBAModel.Convert(a.At(x, y)).(color.NRGBA)
			cb := color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA)
			if ca.A == 0 && cb.A == 0 {
				continue
			}
			if ca != cb {
				return false
			}
		}
	}
	return true
}

func fixtures(t *testing.T) (graph.Input, []string) {
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	var ids []string
	for fi, name := range []string{"synthetic-two-runs.txt", "synthetic-tab.txt", "synthetic-semicolon-comma.txt"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", name))
		if err != nil {
			t.Fatal(err)
		}
		r := lightscattering.Parse(b)
		fid := "f" + string(rune('a'+fi))
		sum := sha256.Sum256(b)
		in.Files[fid] = model.SourceFile{ID: fid, Name: name, SHA256: hex.EncodeToString(sum[:])}
		for i, m := range r.Measurements {
			m.ID = fid + string(rune('0'+i))
			m.FileID = fid
			in.Measurements[m.ID] = m
			ids = append(ids, m.ID)
		}
	}
	return in, ids
}

func TestMeasurementDataPreservesValues(t *testing.T) {
	in, ids := fixtures(t)
	ds := MeasurementData("A1", ids, in, nil, tr)
	ds.AddCommonMeta(tr, "MNE Lab test", 1)
	main := ds.Tables[0]
	col := func(key string) int {
		for i, c := range main.Columns {
			if c.Key == key {
				return i
			}
		}
		t.Fatalf("missing column %s", key)
		return -1
	}
	d, it, ed := col("diameter"), col("intensity"), col("effective_diameter")
	if main.Columns[d].Unit != "nm" {
		t.Fatalf("diameter unit %q", main.Columns[d].Unit)
	}
	// Every exported bin equals the parsed value and keeps its decimals.
	m := in.Measurements[ids[0]]
	dc, ic := m.Dist.Column("diameter"), m.Dist.Column("intensity")
	for i := range dc.Values {
		row := main.Rows[i]
		if *row[d].Num != dc.Values[i] || *row[it].Num != ic.Values[i] {
			t.Fatalf("row %d altered", i)
		}
		want := strings.Replace(ic.Raw[i], ",", ".", 1)
		if got := row[it].String("."); got != want {
			t.Fatalf("decimals changed: %q → %q", ic.Raw[i], got)
		}
	}
	if main.Rows[0][ed].String(".") != m.Params[model.EffectiveDiameter].Raw {
		t.Fatalf("effective diameter %q vs %q", main.Rows[0][ed].String("."), m.Params[model.EffectiveDiameter].Raw)
	}
	for _, f := range []string{"csv", "tsv", "txt", "json", "xlsx"} {
		b, err := WriteData(ds, f, DataOptions{})
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v", f, err)
		}
		switch f {
		case "csv":
			r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
			recs, err := r.ReadAll()
			if err != nil || len(recs) != len(main.Rows)+1 || recs[0][d] != "Diameter (nm)" {
				t.Fatalf("csv: %v %d", err, len(recs))
			}
			if recs[1][it] != strings.Replace(ic.Raw[0], ",", ".", 1) {
				t.Fatal("csv value")
			}
		case "json":
			var v map[string]any
			if err := json.Unmarshal(b, &v); err != nil {
				t.Fatal(err)
			}
		case "xlsx":
			x, err := excelize.OpenReader(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			sheets := x.GetSheetList()
			if len(sheets) != 4 || sheets[0] != "Data" {
				t.Fatalf("sheets %v", sheets)
			}
			v, _ := x.GetCellValue("Data", "A1")
			if v != "Sample ID" {
				t.Fatalf("header %q", v)
			}
			props, _ := x.GetDocProps()
			if props.Creator != "MNE Lab" {
				t.Fatalf("creator %q", props.Creator)
			}
		}
	}
	// Comma-decimal option for spreadsheet programs in such locales.
	b, _ := WriteData(ds, "csv", DataOptions{Decimal: ","})
	if !bytes.Contains(b, []byte(";")) {
		t.Fatal("semicolon CSV")
	}
}

func TestParameterData(t *testing.T) {
	in, ids := fixtures(t)
	start := in.Measurements[ids[0]].MeasuredAt.Time
	cfg := cycle.Config{Name: "c", Start: start, Interval: 1, Unit: cycle.Days, Duration: 3}
	pts, _ := cfg.Points()
	var cands []cycle.Candidate
	for _, id := range ids {
		cands = append(cands, cycle.Candidate{ID: id, MeasuredAt: in.Measurements[id].MeasuredAt})
	}
	as := cycle.Associate(cfg, pts, cands)
	ds, err := ParameterData("Eff", model.EffectiveDiameter, in, graph.CycleInput{Config: cfg, Points: pts, Assignments: as}, tr)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Tables) != 2 || len(ds.Tables[1].Rows) != len(pts) {
		t.Fatalf("statistics rows %d", len(ds.Tables[1].Rows))
	}
	if ds.Tables[1].Rows[0][0].Text != "Day 0" {
		t.Fatalf("point label %q", ds.Tables[1].Rows[0][0].Text)
	}
	for _, f := range []string{"csv", "xlsx", "json", "txt", "tsv"} {
		if _, err := WriteData(ds, f, DataOptions{}); err != nil {
			t.Fatal(f, err)
		}
	}
}

func TestCSVFormulaSafety(t *testing.T) {
	ds := DataSet{Tables: []Table{{Columns: []Column{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}, Rows: [][]Cell{{text("=HYPERLINK(\"x\")"), num(-5, 0)}, {text("-20C"), text("@x")}}}}}
	b, _ := WriteData(ds, "csv", DataOptions{})
	s := string(b)
	if !strings.Contains(s, `"'=HYPERLINK(""x"")"`) || !strings.Contains(s, ",-5") || !strings.Contains(s, "-20C,'@x") {
		t.Fatalf("csv safety: %s", s)
	}
}

func TestNames(t *testing.T) {
	d := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	p, total := CycleTokens("weeks", 4, 4)
	cases := []struct {
		n    NameInfo
		want string
	}{
		{NameInfo{Samples: []string{"A1"}, Module: "DLS", Date: d}, "A1_DLS_2026-10-05"},
		{NameInfo{Samples: []string{"A1"}, Module: "DLS", Point: p}, "A1_DLS_Cycle_Week4"},
		{NameInfo{Samples: []string{"A1", "A1"}, Subject: "Stability", Duration: total}, "A1_Stability_4Weeks"},
		{NameInfo{Samples: []string{"B", "A", "C"}, Module: "DLS", Title: "Graph"}, "A+2_DLS"},
		{NameInfo{Title: "Estabilidade: lote 3/4"}, "Estabilidade-lote_3-4"},
		{NameInfo{Samples: []string{"F-A1"}, Subject: "Stability", Title: "F-A1 stability"}, "F-A1_stability"},
		{NameInfo{Samples: []string{"F-A1"}, Title: "Lote 2"}, "F-A1_Lote_2"},
	}
	for _, c := range cases {
		if got := Suggest(c.n); got != c.want {
			t.Fatalf("Suggest(%+v) = %q, want %q", c.n, got, c.want)
		}
	}
	if Sanitize("CON") != "_CON" || Sanitize("  ..  ") != "MNE-Lab_export" || Sanitize("a\x00b") != "a-b" || Sanitize("lote 3.") != "lote 3" {
		t.Fatal("sanitize")
	}
	// A typed name keeps spaces and accents; only refused characters change.
	if Sanitize("Estabilidade do lote") != "Estabilidade do lote" || Adjusted("Estabilidade do lote") || !Adjusted("lote 3/4") {
		t.Fatal("typed names")
	}
	if WithExt("graph.png", ".svg") != "graph.svg" || WithExt("x.jpeg", ".jpg") != "x.jpeg" || WithExt("lote 1.2", ".csv") != "lote 1.2.csv" {
		t.Fatalf("ext: %s %s %s", WithExt("graph.png", ".svg"), WithExt("x.jpeg", ".jpg"), WithExt("lote 1.2", ".csv"))
	}
}

func TestSaveCollisionsAndAtomicity(t *testing.T) {
	dir := t.TempDir()
	s, err := Save(dir, "A1.csv", CollisionAsk, 10, Bytes([]byte("one")))
	if err != nil || s.Size != 3 {
		t.Fatal(err)
	}
	if _, err := Save(dir, "A1.csv", CollisionAsk, 10, Bytes([]byte("two"))); err != ErrExists {
		t.Fatalf("silent overwrite: %v", err)
	}
	if _, err := Save(dir, "A1.csv", CollisionCancel, 10, Bytes([]byte("two"))); err != ErrCanceled {
		t.Fatal("cancel")
	}
	k, err := Save(dir, "A1.csv", CollisionKeepBoth, 10, Bytes([]byte("two")))
	if err != nil || k.Name != "A1 (2).csv" {
		t.Fatalf("keep both: %v %s", err, k.Name)
	}
	if _, err := Save(dir, "A1.csv", CollisionReplace, 10, Bytes([]byte("three"))); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "A1.csv")); string(b) != "three" {
		t.Fatal("replace")
	}
	// A failing writer leaves nothing behind.
	_, err = Save(dir, "broken.png", CollisionAsk, 10, func(w io.Writer) error {
		w.Write([]byte("partial"))
		return io.ErrUnexpectedEOF
	})
	if err == nil || Exists(dir, "broken.png") {
		t.Fatal("broken export left a file")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("leftover files: %d", len(entries))
	}
	if _, err := Save(dir, "../escape.csv", CollisionAsk, 1, Bytes(nil)); err != ErrInvalidName {
		t.Fatal("path traversal")
	}
	if _, err := Save(filepath.Join(dir, "missing"), "x.csv", CollisionAsk, 1, Bytes(nil)); err != ErrDestination {
		t.Fatal("missing destination")
	}
}

func TestResearchPackage(t *testing.T) {
	a := NewArchive("A1_Stability_4Weeks", KindCycle, "A1", "MNE Lab test", 1, time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
	a.Readme = "readme"
	a.Add(DirOriginals, "run 1.txt", "original", []byte("raw bytes"))
	a.Add(DirOriginals, "run 1.txt", "original", []byte("other raw bytes"))
	a.AddJSON(DirMetadata, "cycle.json", "metadata", map[string]int{"duration": 4})
	var b bytes.Buffer
	m, err := a.WriteZip(&b)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	for _, want := range []string{"A1_Stability_4Weeks/README.txt", "A1_Stability_4Weeks/original-files/run 1.txt", "A1_Stability_4Weeks/original-files/run 1 (2).txt",
		"A1_Stability_4Weeks/manifest/manifest.json", "A1_Stability_4Weeks/manifest/checksums.sha256", "A1_Stability_4Weeks/metadata/cycle.json"} {
		if names[want] == nil {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// Originals are byte-identical and their checksums are listed.
	rc, _ := names["A1_Stability_4Weeks/original-files/run 1.txt"].Open()
	raw, _ := io.ReadAll(rc)
	sum := sha256.Sum256(raw)
	listed := false
	for _, e := range m.Files {
		if e.Path == "original-files/run 1.txt" && e.SHA256 == hex.EncodeToString(sum[:]) && e.Role == "original" {
			listed = true
		}
	}
	if string(raw) != "raw bytes" || !listed {
		t.Fatalf("original bytes or checksum: %+v", m.Files)
	}
	rc, _ = names["A1_Stability_4Weeks/manifest/manifest.json"].Open()
	mb, _ := io.ReadAll(rc)
	for _, forbidden := range []string{"user", "email", "device", "machine", "host"} {
		if bytes.Contains(bytes.ToLower(mb), []byte(forbidden)) {
			t.Fatalf("manifest mentions %q", forbidden)
		}
	}
}
