package convert

import (
	"context"
	"errors"
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

// skipCollisionPreset collides on existing outputs with the safe default.
func skipCollisionPreset() preset.Preset {
	p := encodePreset()
	p.Output.OnCollision = preset.CollisionSkip
	return p
}

// collidingInput writes an input PNG plus a pre-existing .jxl output so the
// run hits an output-exists collision.
func collidingInput(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	pngFile(t, p)
	out := filepath.Join(dir, name[:len(name)-len(filepath.Ext(name))]+".jxl")
	if err := os.WriteFile(out, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEngineCollisionNilHandlerSkips(t *testing.T) {
	dir := t.TempDir()
	input := collidingInput(t, dir, "a.png")
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: skipCollisionPreset()})
	sum := e.Run(context.Background(), []string{input})
	if sum.Skipped != 1 || sum.Completed != 0 {
		t.Fatalf("summary = %+v, want 1 skipped", sum)
	}
}

func TestEngineCollisionPromptDecisions(t *testing.T) {
	cases := []struct {
		name         string
		action       CollisionAction
		wantCalls    int
		wantComplete int
		wantSkip     int
	}{
		{"skip", CollisionSkip, 2, 0, 2},
		{"overwrite", CollisionOverwrite, 2, 2, 0},
		{"skip-all", CollisionSkipAll, 1, 0, 2},
		{"overwrite-all", CollisionOverwriteAll, 1, 2, 0},
		{"rename", CollisionRename, 2, 2, 0},
		{"rename-all", CollisionRenameAll, 1, 2, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			inputs := []string{collidingInput(t, dir, "a.png"), collidingInput(t, dir, "b.png")}
			calls := 0
			e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: skipCollisionPreset()})
			e.CollisionHandler = func(_, _ string) CollisionAction {
				calls++
				return tc.action
			}
			sum := e.Run(context.Background(), inputs)
			if sum.Completed != tc.wantComplete || sum.Skipped != tc.wantSkip {
				t.Fatalf("summary = %+v, want completed=%d skipped=%d", sum, tc.wantComplete, tc.wantSkip)
			}
			if calls != tc.wantCalls {
				t.Fatalf("handler calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

// TestEngineCollisionRenameNumbersOutput verifies a rename answer keeps the
// existing file and writes the conversion to a numbered sibling.
func TestEngineCollisionRenameNumbersOutput(t *testing.T) {
	dir := t.TempDir()
	input := collidingInput(t, dir, "a.png")
	var got []FileResult
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: skipCollisionPreset()})
	e.CollisionHandler = func(_, _ string) CollisionAction { return CollisionRename }
	e.OnFile = func(r FileResult) { got = append(got, r) }
	sum := e.Run(context.Background(), []string{input})
	if sum.Completed != 1 || sum.Skipped != 0 {
		t.Fatalf("summary = %+v, want 1 completed", sum)
	}
	if len(got) != 1 {
		t.Fatalf("OnFile called %d times, want 1", len(got))
	}
	want := filepath.Join(dir, "a (1).jxl")
	if got[0].Output != want {
		t.Errorf("output = %q, want numbered sibling %q", got[0].Output, want)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "a.jxl")); err != nil || string(content) != "existing" {
		t.Errorf("existing output was touched: content=%q err=%v", content, err)
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

// TestEngineFileStartEmitted verifies one start event per file, carrying the
// input path, so the UI can show PID/elapsed for in-flight files.
func TestEngineFileStartEmitted(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) string { p := filepath.Join(dir, n); pngFile(t, p); return p }
	inputs := []string{mk("a.png"), mk("b.png")}
	var mu sync.Mutex
	var starts []FileStarted
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 2, Preset: encodePreset()})
	e.OnFileStart = func(s FileStarted) {
		mu.Lock()
		starts = append(starts, s)
		mu.Unlock()
	}
	sum := e.Run(context.Background(), inputs)
	if sum.Completed != 2 {
		t.Fatalf("summary = %+v, want 2 completed", sum)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(starts) < 2 {
		t.Fatalf("starts = %d, want at least 2", len(starts))
	}
	seen := map[string]bool{}
	for _, s := range starts {
		seen[s.Input] = true
		if s.StartedAt.IsZero() {
			t.Errorf("start for %s has zero StartedAt", s.Input)
		}
	}
	for _, in := range inputs {
		if !seen[in] {
			t.Errorf("no start event for %s", in)
		}
	}
}

// TestEngineCancelFile verifies that cancelling one file leaves the other to
// finish, and that unknown paths report false.
func TestEngineCancelFile(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) string { p := filepath.Join(dir, n); pngFile(t, p); return p }
	blocked, quick := mk("blocked.png"), mk("quick.png")
	enc := &fakeEncoder{}
	// Block only one input; let the other finish instantly.
	e := New(Deps{Encoder: &blockOneEncoder{enc: enc, blockInput: blocked}}, Settings{Processes: 2, Preset: encodePreset()})
	var mu sync.Mutex
	var started []string
	var results []FileResult
	e.OnFileStart = func(s FileStarted) {
		mu.Lock()
		started = append(started, s.Input)
		mu.Unlock()
	}
	e.OnFile = func(r FileResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}
	e.Start(context.Background())
	e.Add([]string{blocked, quick})
	e.CloseInput()
	// Wait until both files are in flight, then cancel only the blocked one.
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(started)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for both files to start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !e.CancelFile(blocked) {
		t.Error("CancelFile(blocked) = false, want true")
	}
	if e.CancelFile(filepath.Join(dir, "missing.png")) {
		t.Error("CancelFile(missing) = true, want false")
	}
	sum := e.Wait()
	if sum.Completed != 1 {
		t.Errorf("completed = %d, want 1 (quick only)", sum.Completed)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, r := range results {
		if r.Input == blocked && !r.Cancelled {
			t.Errorf("blocked result = %+v, want Cancelled", r)
		}
	}
}

// TestEngineResultCarriesArgs verifies the resolved args ride along for the
// settings chips that distinguish repeat runs of one file.
func TestEngineResultCarriesArgs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	var got []FileResult
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: encodePreset()})
	e.OnFile = func(r FileResult) { got = append(got, r) }
	sum := e.Run(context.Background(), []string{p})
	if sum.Completed != 1 {
		t.Fatalf("summary = %+v, want 1 completed", sum)
	}
	if len(got) != 1 || len(got[0].Args) == 0 {
		t.Fatalf("result args = %+v, want resolved encoder args", got)
	}
	found := false
	for _, a := range got[0].Args {
		if a.Key == "-e" && a.Value == "7" {
			found = true
		}
	}
	if !found {
		t.Errorf("result args = %+v, want -e 7", got[0].Args)
	}
}

// blockOneEncoder blocks until ctx cancellation for one input, delegating the
// rest to the wrapped fake.
type blockOneEncoder struct {
	enc        *fakeEncoder
	blockInput string
}

func (b *blockOneEncoder) Run(ctx context.Context, args []cjxl.Arg, input, output string) cjxl.Result {
	if input == b.blockInput {
		<-ctx.Done()
		return cjxl.Result{ExitCode: -1, Err: ctx.Err()}
	}
	return b.enc.Run(ctx, args, input, output)
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

// TestEngineTryAddAfterDrainReturnsFalse verifies that TryAdd reports false
// once all workers have exited, so a caller can start a fresh run instead of
// queueing into a dead engine.
func TestEngineTryAddAfterDrainReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) string { p := filepath.Join(dir, n); pngFile(t, p); return p }
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: encodePreset()})
	e.Start(context.Background())
	e.Add([]string{mk("a.png")})
	e.CloseInput()
	sum := e.Wait()
	if sum.Completed != 1 {
		t.Fatalf("completed = %d, want 1", sum.Completed)
	}
	// Engine is done; TryAdd must refuse.
	if e.TryAdd([]string{mk("b.png")}) {
		t.Fatal("TryAdd returned true after the engine drained")
	}
}

// TestEngineTryAddWhileRunningReturnsTrue verifies that TryAdd accepts work
// while the engine is still processing.
func TestEngineTryAddWhileRunningReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	mk := func(n string) string { p := filepath.Join(dir, n); pngFile(t, p); return p }
	e := New(Deps{Encoder: &fakeEncoder{blockCtx: true}}, Settings{Processes: 1, Preset: encodePreset()})
	e.Start(context.Background())
	e.Add([]string{mk("a.png")})
	// Worker is blocked on a.png; TryAdd should still accept.
	if !e.TryAdd([]string{mk("b.png")}) {
		t.Fatal("TryAdd returned false while the engine is running")
	}
	e.Cancel()
	_ = e.Wait()
}

func TestEngineEmbedsSettingsInFilename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	ps := encodePreset()
	ps.Output.EmbedSettings = true
	ps.Rules[0].Args = []cjxl.Arg{{Key: "-d", Value: "1"}, {Key: "-e", Value: "7"}}
	var done []FileResult
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: ps, CJXLVersion: "0.11.1"})
	e.OnFile = func(r FileResult) { done = append(done, r) }
	sum := e.Run(context.Background(), []string{p})
	if sum.Completed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(done) != 1 {
		t.Fatalf("OnFile called %d times", len(done))
	}
	want := filepath.Join(dir, "a.d1.00-e7-cjxl0.11.1.jxl")
	if done[0].Output != want {
		t.Errorf("output = %q, want %q", done[0].Output, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("missing suffixed output: %v", err)
	}
}

// fakeInspector records inspected paths and returns canned metadata.
type fakeInspector struct {
	info  string
	err   error
	calls []string
}

func (f *fakeInspector) Inspect(_ context.Context, path string) (string, error) {
	f.calls = append(f.calls, path)
	return f.info, f.err
}

func sidecarPreset() preset.Preset {
	ps := encodePreset()
	ps.Output.JXLInfoSidecar = true
	ps.Rules[0].Args = []cjxl.Arg{{Key: "-d", Value: "0.5"}, {Key: "-e", Value: "7"}}
	return ps
}

func TestEngineWritesJXLInfoSidecar(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	inspector := &fakeInspector{info: "meta"}
	var done []FileResult
	e := New(Deps{Encoder: &fakeEncoder{}, Inspector: inspector}, Settings{Processes: 1, Preset: sidecarPreset()})
	e.OnFile = func(r FileResult) { done = append(done, r) }
	sum := e.Run(context.Background(), []string{p})
	if sum.Completed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	sidecar := filepath.Join(dir, "a.jxl.jxlinfo.txt")
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("missing sidecar: %v", err)
	}
	if string(data) != "meta\n" {
		t.Errorf("sidecar = %q", data)
	}
	if len(done) != 1 || done[0].Warning != "" {
		t.Errorf("result = %+v, want no warning", done)
	}
	if len(inspector.calls) != 1 {
		t.Fatalf("inspect calls = %d, want 1", len(inspector.calls))
	}
}

func TestEngineSidecarOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	sidecar := filepath.Join(dir, "a.jxl.jxlinfo.txt")
	if err := os.WriteFile(sidecar, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(Deps{Encoder: &fakeEncoder{}, Inspector: &fakeInspector{info: "new"}}, Settings{Processes: 1, Preset: sidecarPreset()})
	if sum := e.Run(context.Background(), []string{p}); sum.Completed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Errorf("sidecar = %q, want overwrite", data)
	}
}

func TestEngineSidecarFailureWarns(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	e := New(
		Deps{Encoder: &fakeEncoder{}, Inspector: &fakeInspector{err: errors.New("boom")}},
		Settings{Processes: 1, Preset: sidecarPreset()},
	)
	var done []FileResult
	e.OnFile = func(r FileResult) { done = append(done, r) }
	sum := e.Run(context.Background(), []string{p})
	if sum.Completed != 1 || sum.Failed != 0 {
		t.Fatalf("sidecar failure should not fail the conversion: %+v", sum)
	}
	if len(done) != 1 || done[0].Warning == "" {
		t.Fatalf("result = %+v, want a warning", done)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.jxl.jxlinfo.txt")); !os.IsNotExist(err) {
		t.Error("failed sidecar should leave no file behind")
	}
}

func TestEngineSidecarNilInspectorWarns(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	pngFile(t, p)
	e := New(Deps{Encoder: &fakeEncoder{}}, Settings{Processes: 1, Preset: sidecarPreset()})
	var done []FileResult
	e.OnFile = func(r FileResult) { done = append(done, r) }
	if sum := e.Run(context.Background(), []string{p}); sum.Completed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(done) != 1 || done[0].Warning == "" {
		t.Errorf("result = %+v, want a missing-inspector warning", done)
	}
}
