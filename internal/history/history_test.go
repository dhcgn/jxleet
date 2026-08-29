package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTripAndClear(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "nested", "history.jsonl"))

	entries, err := store.List()
	if err != nil {
		t.Fatalf("List on missing file: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List on missing file returned %d entries", len(entries))
	}

	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	if err := store.Append(Entry{At: at, Input: `C:\a.jpg`, Output: `C:\a.jxl`, Route: "Transcode", Preset: "p", InputSize: 100, OutputSize: 80}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(Entry{At: at, Input: `C:\b.png`, Output: `C:\b.jxl`, Route: "Encode", Preset: "p", InputSize: 200, OutputSize: 50}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err = store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}
	if entries[0].Input != `C:\a.jpg` || entries[0].OutputSize != 80 || entries[1].Input != `C:\b.png` {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if !entries[0].At.Equal(at) {
		t.Fatalf("timestamp not preserved: %v", entries[0].At)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	entries, err = store.List()
	if err != nil {
		t.Fatalf("List after Clear: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List after Clear returned %d entries", len(entries))
	}
}

// TestStoreListSkipsCorruptLine simulates a torn write at the tail (killed
// mid-append) and verifies the readable entries still load.
func TestStoreListSkipsCorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := New(path)
	if err := store.Append(Entry{Input: `C:\a.jpg`, Output: `C:\a.jxl`, InputSize: 1, OutputSize: 1}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := os.WriteFile(path, append(mustReadFile(t, path), []byte(`{"input":`+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := store.List()
	if err != nil {
		t.Fatalf("List with corrupt tail: %v", err)
	}
	if len(entries) != 1 || entries[0].Input != `C:\a.jpg` {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
