package app

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/web"
	"github.com/xuri/excelize/v2"
)

// TestBrowserHarness serves a seeded MNE Lab (synthetic measurements, a
// cycle and graphs) for the browser tests in web/tests/e2e. scripts/e2e.sh
// runs both; normal test runs skip it.
//
// MNELAB_E2E_DIR is a working folder. The harness writes base.txt when it
// is ready, answers each want-url file with a fresh one-time launch URL in
// url.txt, and stops when a stop file appears. MNELAB_E2E_MODE=temporary
// starts Temporary Machine Mode instead of Portable USB Mode.
func TestBrowserHarness(t *testing.T) {
	dir := os.Getenv("MNELAB_E2E_DIR")
	if dir == "" {
		t.Skip("browser harness (scripts/e2e.sh)")
	}
	// The phone runs on the same machine in the tests.
	lanIPFunc = func() (net.IP, error) { return net.IPv4(127, 0, 0, 1), nil }
	root := filepath.Join(dir, "root")
	home := filepath.Join(dir, "home")
	os.MkdirAll(root, 0o755)
	os.MkdirAll(home, 0o755)
	var opt Options
	if os.Getenv("MNELAB_E2E_MODE") == "temporary" {
		opt = Options{Exe: filepath.Join(home, "Downloads", "mnelab"), ForceMode: paths.Temporary, Headless: true, KDF: fastKDF, BaseTemp: filepath.Join(dir, "tmp"), UserHome: home}
	} else {
		os.WriteFile(filepath.Join(root, paths.PortableMarker), nil, 0o644)
		opt = Options{Exe: filepath.Join(root, "app", "0.1.0", "mnelab"), Root: root, Headless: true, KDF: fastKDF, UserHome: home}
	}
	a, err := New(opt)
	if err != nil {
		t.Fatal(err)
	}
	ui, ok := web.FS()
	if !ok {
		t.Fatal("the interface is not built (cd web && npm run build)")
	}
	a.ui = ui
	srv, err := newServer(a, ui)
	if err != nil {
		t.Fatal(err)
	}
	a.srv = srv
	defer a.Close()
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: "http://" + srv.host, hc: &http.Client{Jar: jar, Timeout: 60 * time.Second}}
	res, err := c.hc.Get(srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if os.Getenv("MNELAB_E2E_EMPTY") == "" {
		seed(t, c)
	}
	workbook := excelize.NewFile()
	workbook.SetSheetName("Sheet1", "Synthetic sheet")
	for i, row := range [][]any{
		{"Sample ID", "Synthetic workbook"}, {"Effective Diameter (nm)", 123.456},
		{"Date", "2026-09-18 14:30:00"}, {"Diameter (nm)", "Intensity (%)"},
		{10, 10}, {20, 20}, {30, 70},
	} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := workbook.SetSheetRow("Synthetic sheet", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SaveAs(filepath.Join(dir, "synthetic.xlsx")); err != nil {
		t.Fatal(err)
	}
	workbook.NewSheet("Other sheet")
	workbook.SetCellValue("Other sheet", "A1", "Effective Diameter (nm)")
	workbook.SetCellValue("Other sheet", "B1", 321.500)
	workbook.NewSheet("Empty sheet")
	if err := workbook.SaveAs(filepath.Join(dir, "synthetic-selection.xlsx")); err != nil {
		t.Fatal(err)
	}
	workbook.Close()
	methods, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-methods.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "synthetic-methods.txt"), methods, 0o600); err != nil {
		t.Fatal(err)
	}
	counts, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-count-rate-types.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "synthetic-count-rates.txt"), counts, 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "base.txt"), []byte(c.base), 0o644)
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "want-url")); err == nil {
			os.Remove(filepath.Join(dir, "want-url"))
			os.WriteFile(filepath.Join(dir, "url.txt"), []byte(srv.LaunchURL("/")), 0o644)
		}
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// synth writes a NanoBrook-like export with a log-normal distribution.
func synth(sample string, at time.Time, mean, pdi float64, seed int) []byte {
	s := fmt.Sprintf("NanoBrook 90Plus\nSample ID:\t%s\nDate:\t%s\nEffective Diameter (nm):\t%.2f\nPolydispersity:\t%.3f\nCurrent Count Rate:\t%.1f kcps\nBaseLine Index:\t%.1f\n\nDiameter (nm)\tIntensity (%%)\tVolume (%%)\tNumber (%%)\n",
		sample, at.Format("2006-01-02 15:04:05"), mean, pdi, 300+float64(seed%7)*4, 8.5+float64(seed%3)*0.3)
	sig := 0.25 + pdi
	type row struct{ d, i, v, n float64 }
	var rows []row
	var si, sv, sn float64
	for k := 0; k < 40; k++ {
		d := 10 * math.Pow(1.15, float64(k))
		g := func(m float64) float64 { x := math.Log(d / m); return math.Exp(-x * x / (2 * sig * sig)) }
		r := row{d, g(mean), g(mean * 0.8), g(mean * 0.6)}
		si, sv, sn = si+r.i, sv+r.v, sn+r.n
		rows = append(rows, r)
	}
	for _, r := range rows {
		s += fmt.Sprintf("%.2f\t%.4f\t%.4f\t%.4f\n", r.d, 100*r.i/si, 100*r.v/sv, 100*r.n/sn)
	}
	return []byte(s)
}

func prof(name, sm, provider string, color int) map[string]any {
	m := map[string]any{"username": name, "storageMode": sm, "color": color}
	if sm != "usb_only" {
		m["provider"] = provider
	}
	return m
}

func seed(t *testing.T, c *client) {
	sm := "usb_only"
	if os.Getenv("MNELAB_E2E_MODE") == "temporary" {
		sm = "cloud_only"
	}
	c.json("POST", "/api/account/create", map[string]any{"name": "Lab", "passphrase": "correct horse battery staple"}, nil)
	var pv ProfileView
	c.json("POST", "/api/profiles", prof("Ana", sm, "onedrive", 0), &pv)
	c.json("POST", "/api/profiles", prof("Bruno", sm, "google", 3), nil)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)
	// The synthetic reports declare a wall clock without a zone. The cycle
	// must carry the same unknown-zone semantics, independent of the host TZ.
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	n := 0
	for day := 0; day <= 28; day += 7 {
		for r := 1; r <= 3; r++ {
			n++
			mean := 180 + float64(day)*1.6 + float64(r)*2.1
			data := synth("F-A1", start.Add(time.Duration(day)*24*time.Hour+time.Duration(r)*10*time.Minute), mean, 0.12+float64(day)*0.002, n)
			code, out := c.do("POST", "/api/files", data, map[string]string{"X-File-Name": url.QueryEscape(fmt.Sprintf("F-A1 day %d rep %d.txt", day, r))})
			if code != 200 {
				t.Fatalf("import %d %s", code, out)
			}
		}
	}
	for _, f := range []string{"synthetic-tab.txt", "synthetic-two-runs.txt", "synthetic-semicolon-comma.txt", "synthetic-missing-fields.txt", "synthetic-ambiguous-date.txt"} {
		data, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", f))
		c.do("POST", "/api/files", data, map[string]string{"X-File-Name": f})
	}
	var files []FileView
	c.json("GET", "/api/files", nil, &files)
	var cyc []string
	var cmp []string
	for _, f := range files {
		for _, it := range f.Items {
			if it.SampleID == "F-A1" {
				cyc = append(cyc, it.ID)
			} else if len(cmp) < 3 {
				cmp = append(cmp, it.ID)
			}
		}
	}
	c.json("POST", "/api/graphs", map[string]any{"kind": "dls_distribution", "title": "F-A1 day 0", "measurements": cyc[:1], "visual": map[string]any{"legend": true, "grid": true, "lineWidth": 1.75, "fontScale": 1}}, nil)
	c.json("POST", "/api/graphs", map[string]any{"kind": "dls_distribution", "title": "Comparison", "measurements": append(cmp, cyc[0], cyc[len(cyc)-1]), "visual": map[string]any{"legend": true, "grid": true, "lineWidth": 1.75, "fontScale": 1}}, nil)
	var cy struct {
		ID string `json:"id"`
	}
	c.json("POST", "/api/cycles", map[string]any{"config": map[string]any{"name": "F-A1 stability", "sampleId": "F-A1", "start": start.UTC().Format(time.RFC3339), "startTzKnown": false, "interval": 7, "unit": "days", "duration": 28, "replicates": 3, "params": []string{"effective_diameter", "polydispersity", "count_rate", "baseline_index"}}, "measurements": cyc}, &cy)
	c.json("POST", "/api/graphs", map[string]any{"kind": "parameter_time", "title": "Effective diameter over 28 days", "cycleId": cy.ID, "param": "effective_diameter", "measurements": cyc, "visual": map[string]any{"legend": true, "grid": true, "lineWidth": 1.75, "fontScale": 1}}, nil)
	c.json("POST", "/api/graphs", map[string]any{"kind": "dls_by_time", "title": "Distribution over the cycle", "cycleId": cy.ID, "measurements": cyc, "visual": map[string]any{"legend": true, "grid": true, "lineWidth": 1.75, "fontScale": 1}}, nil)
}
