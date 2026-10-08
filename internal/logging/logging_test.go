package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedaction(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelDebug)
	l.Info("x", "password", "hunter2", "accessToken", "abc", "note", "Bearer abcdefghijklmnopqrstuvwxyz0123456789ABCD", "count", 3)
	out := buf.String()
	for _, leak := range []string{"hunter2", "\"abc\"", "abcdefghijklmnopqrstuvwxyz0123456789ABCD"} {
		if strings.Contains(out, leak) {
			t.Fatalf("secret leaked: %s", out)
		}
	}
	if !strings.Contains(out, `"count":3`) {
		t.Fatalf("harmless value dropped: %s", out)
	}
}

func TestRotationBounded(t *testing.T) {
	d := t.TempDir()
	r, err := OpenRotating(d, "app")
	if err != nil {
		t.Fatal(err)
	}
	line := bytes.Repeat([]byte("x"), 4096)
	for i := 0; i < 2000; i++ {
		r.Write(line)
	}
	r.Close()
	var total int64
	entries, _ := os.ReadDir(d)
	for _, e := range entries {
		st, _ := os.Stat(filepath.Join(d, e.Name()))
		total += st.Size()
	}
	if len(entries) > keepFiles+1 || total > int64(keepFiles+1)*maxFileBytes {
		t.Fatalf("logs not bounded: %d files, %d bytes", len(entries), total)
	}
}
