// Package history persists a per-session-independent record of successful
// conversions as an append-only JSONL file. One JSON object per line keeps
// appends cheap (no rewrite of the file) and tolerates a torn last line.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one successful conversion.
type Entry struct {
	At         time.Time `json:"at"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	Route      string    `json:"route"`
	Preset     string    `json:"preset"`
	InputSize  int64     `json:"inputSize"`
	OutputSize int64     `json:"outputSize"`
}

// Store appends and reads entries from a single JSONL file.
type Store struct {
	path string
	mu   sync.Mutex
}

// New returns a Store writing to path; the file is created on first Append.
func New(path string) *Store {
	return &Store{path: path}
}

// Append writes one entry as a new line.
func (s *Store) Append(e Entry) error {
	if e.Input == "" || e.Output == "" {
		return errors.New("history: entry needs input and output")
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("history: encode entry: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("history: create directory: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("history: open: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("history: append: %w", err)
	}
	return nil
}

// List reads all entries, oldest first. A corrupt line (e.g. a killed append)
// is skipped instead of failing the whole list.
func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("history: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // tolerate a torn write at the tail
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("history: read: %w", err)
	}
	return entries, nil
}

// Clear truncates the history file.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path, nil, 0o644); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("history: clear: %w", err)
	}
	return nil
}
