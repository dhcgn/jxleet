package convert

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/preset"
)

// fakeEncoder writes the temp output and records the args it was called with.
type fakeEncoder struct {
	mu       sync.Mutex
	callArgs [][]cjxl.Arg
	fail     bool
	blockCtx bool // block until the context is cancelled
}

func (f *fakeEncoder) Run(ctx context.Context, args []cjxl.Arg, _ /*input*/, output string) cjxl.Result {
	if f.blockCtx {
		<-ctx.Done()
		return cjxl.Result{ExitCode: -1, Err: ctx.Err()}
	}
	f.mu.Lock()
	f.callArgs = append(f.callArgs, args)
	f.mu.Unlock()
	if f.fail {
		return cjxl.Result{ExitCode: 1, Stderr: "boom"}
	}
	_ = os.WriteFile(output, []byte("jxl"), 0o644)
	return cjxl.Result{ExitCode: 0}
}

func pngFile(t *testing.T, path string) {
	t.Helper()
	magic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(path, append(magic, []byte("payloaddata")...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func encodePreset() preset.Preset {
	return preset.Preset{
		Name:    "t",
		Version: 1,
		Output:  preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionOverwrite},
		Rules:   []preset.Rule{{Match: []string{"*"}, Args: []cjxl.Arg{{Key: "-e", Value: "7"}}}},
	}
}

func TestEngineBasicBatch(t *testing.T) {
	dir := t.TempDir()
	var inputs []string
	for _, n := range []string{"a.png", "b.png", "c.png"} {
		p := filepath.Join(dir, n)
		pngFile(t, p)
		inputs = append(inputs, p)
	}

	var mu sync.Mutex
	var done []FileResult
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 2, Preset: encodePreset()})
	e.OnFile = func(r FileResult) { mu.Lock(); done = append(done, r); mu.Unlock() }

	sum := e.Run(context.Background(), inputs)
	if sum.Completed != 3 || sum.Failed != 0 || sum.Skipped != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(done) != 3 {
		t.Fatalf("OnFile called %d times", len(done))
	}
	for _, in := range inputs {
		out := in[:len(in)-len(".png")] + ".jxl"
		if _, err := os.Stat(out); err != nil {
			t.Errorf("missing output %s", out)
		}
	}
}

func TestEngineSkipsUnsupported(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Preset: encodePreset()})
	sum := e.Run(context.Background(), []string{txt})
	if sum.Skipped != 1 || sum.Completed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestEngineFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	e := New(Deps{Encoder: &fakeEncoder{fail: true}}, Settings{Preset: encodePreset()})
	var result FileResult
	e.OnFile = func(r FileResult) { result = r }
	sum := e.Run(context.Background(), []string{p})
	if sum.Failed != 1 {
		t.Fatalf("expected 1 failure, got %+v", sum)
	}
	if result.Output != "" {
		t.Fatalf("failed result should not expose an output path: %q", result.Output)
	}
}

func TestEngineThreadsInjection(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	enc := &fakeEncoder{}
	e := New(Deps{Encoder: enc}, Settings{Threads: 4, Preset: encodePreset()})
	e.Run(context.Background(), []string{p})

	enc.mu.Lock()
	defer enc.mu.Unlock()
	if len(enc.callArgs) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enc.callArgs))
	}
	found := false
	for _, a := range enc.callArgs[0] {
		if a.Key == "--num_threads" && a.Value == "4" {
			found = true
		}
	}
	if !found {
		t.Errorf("--num_threads=4 not injected: %+v", enc.callArgs[0])
	}
}

func TestEngineThreadsRespectsPresetValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	enc := &fakeEncoder{}
	ps := encodePreset()
	ps.Rules[0].Args = append(ps.Rules[0].Args, cjxl.Arg{Key: "--num_threads", Value: "2"})
	e := New(Deps{Encoder: enc}, Settings{Threads: 8, Preset: ps})
	e.Run(context.Background(), []string{p})

	enc.mu.Lock()
	defer enc.mu.Unlock()
	count := 0
	for _, a := range enc.callArgs[0] {
		if a.Key == "--num_threads" {
			count++
			if a.Value != "2" {
				t.Errorf("preset num_threads should win, got %s", a.Value)
			}
		}
	}
	if count != 1 {
		t.Errorf("num_threads should appear once, got %d", count)
	}
}

func TestEngineCancel(t *testing.T) {
	dir := t.TempDir()
	var inputs []string
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		pngFile(t, p)
		inputs = append(inputs, p)
	}
	e := New(Deps{Encoder: &fakeEncoder{blockCtx: true}}, Settings{Processes: 2, Preset: encodePreset()})
	e.Start(context.Background())
	e.Add(inputs)
	e.CloseInput()
	e.Cancel()
	sum := e.Wait()
	if !sum.Cancelled {
		t.Error("summary should be marked cancelled")
	}
	if sum.Completed != 0 {
		t.Errorf("no file should complete when cancelled immediately, got %d", sum.Completed)
	}
}

func TestEngineCoalesce(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) string { p := filepath.Join(dir, n); pngFile(t, p); return p }
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: encodePreset()})
	e.Start(context.Background())
	e.Add([]string{mk("a.png"), mk("b.png")})
	e.Add([]string{mk("c.png"), mk("d.png"), mk("e.png")})
	e.CloseInput()
	sum := e.Wait()
	if sum.Total != 5 || sum.Completed != 5 {
		t.Fatalf("summary = %+v", sum)
	}
	if p := e.Progress(); p.Coalesced != 2 {
		t.Errorf("coalesced = %d, want 2", p.Coalesced)
	}
}

func TestEnginePauseResume(t *testing.T) {
	dir := t.TempDir()
	var inputs []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".png")
		pngFile(t, p)
		inputs = append(inputs, p)
	}
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: encodePreset()})
	e.Start(context.Background())
	e.Pause()
	e.Add(inputs)
	e.CloseInput()

	// While paused, no file is processed even though input is closed.
	time.Sleep(50 * time.Millisecond)
	if p := e.Progress(); p.Completed != 0 {
		t.Fatalf("paused engine processed %d files", p.Completed)
	}

	e.Resume()
	sum := e.Wait()
	if sum.Completed != 3 {
		t.Errorf("after resume, completed = %d, want 3", sum.Completed)
	}
}
