// Package convert is the conversion engine: it turns a stream of input paths
// into cjxl runs, applying the active preset's routes and output policy, with a
// worker pool, pause/cancel, coalescing of late arrivals, and a throughput-based
// ETA (see README "Concurrency" and "Progress").
package convert

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/output"
	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

// Encoder runs a single cjxl encode. *cjxl.Runner satisfies it; tests inject a
// fake.
type Encoder interface {
	Run(ctx context.Context, args []cjxl.Arg, input, output string) cjxl.Result
}

// Inspector returns verbose metadata for a .jxl file. *jxlinfo.Runner
// satisfies it; tests inject a fake.
type Inspector interface {
	Inspect(ctx context.Context, jxlPath string) (string, error)
}

// CollisionAction is the answer to an interactive output-exists prompt.
type CollisionAction int

const (
	// CollisionSkip skips this file only.
	CollisionSkip CollisionAction = iota
	// CollisionSkipAll skips this file and all later collisions without asking.
	CollisionSkipAll
	// CollisionOverwrite overwrites the existing output for this file only.
	CollisionOverwrite
	// CollisionOverwriteAll overwrites now and all later collisions without asking.
	CollisionOverwriteAll
	// CollisionRename keeps the existing output and writes this file to a
	// numbered sibling ("photo (1).jxl") for this file only.
	CollisionRename
	// CollisionRenameAll numbers now and all later collisions without asking.
	CollisionRenameAll
)

// Deps are the external collaborators the engine needs.
type Deps struct {
	Encoder   Encoder         // one cjxl run per file (ref:jl:tech.tool.encode)
	Verifier  output.Verifier // required for the replace policy; may be nil otherwise (ref:jl:domain.output.verify)
	Inspector Inspector       // required for the jxlinfo-sidecar flag; may be nil otherwise (ref:jl:tech.tool.inspect)
}

// Settings configure a run. Processes and Threads are independent: Processes is
// how many cjxl invocations run in parallel, Threads is --num_threads passed to
// each (0 leaves it to the preset / cjxl default). CJXLVersion is the installed
// toolchain version embedded in output filenames when the preset enables it.
type Settings struct {
	Processes   int
	Threads     int
	Preset      preset.Preset
	Deletion    output.DeletionByRoute
	CJXLVersion string
}

// FileResult is the outcome for one input file.
// jl:domain.queue.item=One staged file with a frozen settings snapshot (distance with quality, effort, extra-flags hint); later preset edits never touch it, and the same file may be queued twice with different settings.
type FileResult struct {
	Input      string
	Format     routes.Format
	Route      routes.Route
	Output     string
	InputSize  int64
	OutputSize int64
	Skipped    bool
	SkipReason string
	Cancelled  bool
	Err        error
	Warning    string // non-fatal note, e.g. a failed jxlinfo sidecar
	Duration   time.Duration
	PID        int        // OS pid of the cjxl child, 0 if never started
	Args       []cjxl.Arg // resolved encoder args for this file (for settings display)
	StartedAt  time.Time  // when this file's encode started
}

// FileStarted is emitted when one file's encode begins.
type FileStarted struct {
	Input     string
	PID       int
	StartedAt time.Time
}

// Progress is a snapshot of a running conversion.
type Progress struct {
	Total      int
	Completed  int
	Failed     int
	Skipped    int
	InFlight   int
	BytesTotal int64
	BytesDone  int64
	Throughput float64 // bytes/sec, recent
	ETA        time.Duration
	Coalesced  int // number of Add batches merged into this run
	Paused     bool
}

// Summary is returned when a run finishes.
type Summary struct {
	Total     int
	Completed int
	Failed    int
	Skipped   int
	Cancelled bool
	BytesIn   int64
	BytesOut  int64
	Duration  time.Duration
}

// Engine coordinates the workers and shared state.
type Engine struct {
	deps     Deps
	settings Settings

	// Callbacks are invoked from worker goroutines; keep them fast and
	// non-blocking. Both are optional.
	OnFile      func(FileResult)
	OnFileStart func(FileStarted)
	OnProgress  func(Progress)

	// CollisionHandler is invoked synchronously from a worker when the output
	// file already exists under the skip-on-collision policy. It may block
	// (e.g. waiting on a user decision). Nil keeps the silent-skip behavior.
	CollisionHandler func(input, target string) CollisionAction

	mu       sync.Mutex
	cond     *sync.Cond
	pending  []string
	inflight int

	// fileCancels tracks one cancel func per in-flight file for per-file cancel.
	fileCancels map[string]context.CancelFunc

	total      int
	completed  int
	failed     int
	skipped    int
	bytesTotal int64
	bytesDone  int64
	bytesOut   int64
	coalesced  int

	// collisionAll is set by CollisionSkipAll/CollisionOverwriteAll/
	// CollisionRenameAll answers and short-circuits later prompts for the
	// rest of the run.
	collisionAll CollisionAction

	paused      bool
	cancelled   bool
	inputClosed bool

	// workersExited counts workers that have returned. TryAdd refuses work once
	// all workers have exited so a caller can start a fresh run instead of
	// appending to an engine that will never process the input.
	workersExited int

	ctx    context.Context
	cancel context.CancelFunc

	tp        *throughput
	startTime time.Time
	wg        sync.WaitGroup
}

// New constructs an Engine.
func New(deps Deps, settings Settings) *Engine {
	if settings.Processes < 1 {
		settings.Processes = 1
	}
	e := &Engine{deps: deps, settings: settings, tp: newThroughput(15), fileCancels: make(map[string]context.CancelFunc)}
	e.cond = sync.NewCond(&e.mu)
	return e
}

// Run processes inputs to completion (no further Add expected) and returns a
// Summary. It is a convenience over Start/Add/CloseInput/Wait.
func (e *Engine) Run(ctx context.Context, inputs []string) Summary {
	e.Start(ctx)
	e.Add(inputs)
	e.CloseInput()
	return e.Wait()
}

// Start spawns the worker pool. Workers block until items are added and exit
// once the input is closed and the queue is drained, or on Cancel.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.startTime = time.Now()
	n := e.settings.Processes
	e.mu.Unlock()

	for i := 0; i < n; i++ {
		e.wg.Add(1)
		go e.worker()
	}
}

// Add appends inputs to the queue. Safe to call while running; late arrivals are
// coalesced into the same run and progress bar. Inputs queued after the workers
// have exited are dropped silently — callers that must guarantee processing
// after a run finished should use TryAdd and start a fresh run on false.
func (e *Engine) Add(inputs []string) {
	_ = e.TryAdd(inputs)
}

// Done returns a channel closed when the run is cancelled. Before Start it
// returns an already-closed channel.
func (e *Engine) Done() <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return e.ctx.Done()
}

// TryAdd appends inputs to the queue and reports whether the engine can still
// process them. It returns false once every worker has exited (input closed and
// drained, or cancelled), so a caller can start a new run instead of handing
// paths to a dead engine.
func (e *Engine) TryAdd(inputs []string) bool {
	if len(inputs) == 0 {
		return true
	}
	e.mu.Lock()
	if e.workersExited >= e.settings.Processes {
		e.mu.Unlock()
		return false
	}
	for _, in := range inputs {
		e.pending = append(e.pending, in)
		e.total++
		if fi, err := os.Stat(in); err == nil {
			e.bytesTotal += fi.Size()
		}
	}
	e.coalesced++
	e.cond.Broadcast()
	e.mu.Unlock()
	e.emitProgress()
	return true
}

// CloseInput signals that no more Add calls will come; workers finish the queue
// and exit.
func (e *Engine) CloseInput() {
	e.mu.Lock()
	e.inputClosed = true
	e.cond.Broadcast()
	e.mu.Unlock()
}

// Pause stops dispatching new files; in-flight files finish.
func (e *Engine) Pause() {
	e.mu.Lock()
	e.paused = true
	e.mu.Unlock()
	e.emitProgress()
}

// Resume continues after a Pause.
func (e *Engine) Resume() {
	e.mu.Lock()
	e.paused = false
	e.cond.Broadcast()
	e.mu.Unlock()
	e.emitProgress()
}

// Cancel stops the run, cancelling any in-flight cjxl processes.
func (e *Engine) Cancel() {
	e.mu.Lock()
	e.cancelled = true
	if e.cancel != nil {
		e.cancel()
	}
	e.cond.Broadcast()
	e.mu.Unlock()
}

// CancelFile cancels one in-flight file; queued and finished files are
// unaffected. It reports whether the file was in flight.
func (e *Engine) CancelFile(input string) bool {
	e.mu.Lock()
	cancel, ok := e.fileCancels[input]
	e.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// Wait blocks until all workers have exited and returns the Summary.
func (e *Engine) Wait() Summary {
	e.wg.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	return Summary{
		Total:     e.total,
		Completed: e.completed,
		Failed:    e.failed,
		Skipped:   e.skipped,
		Cancelled: e.cancelled,
		BytesIn:   e.bytesDone,
		BytesOut:  e.bytesOut,
		Duration:  time.Since(e.startTime),
	}
}

// worker pulls files off the queue and processes them until told to stop.
func (e *Engine) worker() {
	defer e.wg.Done()
	defer func() {
		e.mu.Lock()
		e.workersExited++
		// Wake any sibling workers so they observe the exit count and leave
		// once the queue is fully drained.
		e.cond.Broadcast()
		e.mu.Unlock()
	}()
	for {
		path, ok := e.acquire()
		if !ok {
			return
		}
		// One child context per file so CancelFile stops only this file.
		e.mu.Lock()
		fileCtx, fileCancel := context.WithCancel(e.ctx)
		e.fileCancels[path] = fileCancel
		e.mu.Unlock()
		res := e.process(fileCtx, path)
		e.mu.Lock()
		delete(e.fileCancels, path)
		e.mu.Unlock()
		fileCancel()
		e.finish(res)
	}
}

// acquire returns the next path to process, blocking while paused or empty. It
// returns ok=false when the worker should exit (cancelled, or input closed and
// nothing left to do).
func (e *Engine) acquire() (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for {
		if e.cancelled {
			return "", false
		}
		if e.paused {
			e.cond.Wait()
			continue
		}
		if len(e.pending) > 0 {
			path := e.pending[0]
			e.pending = e.pending[1:]
			e.inflight++
			return path, true
		}
		if e.inputClosed && e.inflight == 0 {
			// Drained; wake any siblings also waiting so they can exit too.
			e.cond.Broadcast()
			return "", false
		}
		e.cond.Wait()
	}
}

// finish records a result and wakes waiters.
func (e *Engine) finish(res FileResult) {
	e.mu.Lock()
	e.inflight--
	switch {
	case res.Cancelled:
		// not counted as completed or failed
	case res.Skipped:
		e.skipped++
	case res.Err != nil:
		e.failed++
	default:
		e.completed++
	}
	e.bytesDone += res.InputSize
	e.bytesOut += res.OutputSize
	e.tp.record(e.bytesDone)
	e.cond.Broadcast()
	e.mu.Unlock()

	if e.OnFile != nil {
		e.OnFile(res)
	}
	e.emitProgress()
}

// process runs the full per-file pipeline.
func (e *Engine) process(ctx context.Context, path string) FileResult {
	start := time.Now()
	res := FileResult{Input: path}
	if fi, err := os.Stat(path); err == nil {
		res.InputSize = fi.Size()
	}

	format := detectFormat(path)
	res.Format = format
	if format == routes.FormatUnknown {
		res.Skipped = true
		res.SkipReason = "unsupported format"
		res.Duration = time.Since(start)
		return res
	}

	route, args, ok := e.settings.Preset.Route(format)
	res.Route = route
	if !ok {
		res.Skipped = true
		res.SkipReason = "no matching rule"
		res.Duration = time.Since(start)
		return res
	}

	eff := output.EffectiveOutput(e.settings.Preset.Output, route, e.settings.Deletion)
	suffix := ""
	if eff.EmbedSettings {
		suffix = output.SuffixFor(route, args, e.settings.CJXLVersion)
	}
	plan, err := output.PrepareWithSuffix(path, eff, suffix)
	if err != nil {
		res.Err = err
		res.Duration = time.Since(start)
		return res
	}
	if plan.Skip {
		// plan.Skip only arises from the output-exists collision branch, so an
		// "overwrite" answer re-prepares with the overwrite policy and a
		// "rename" answer re-prepares with the numbering policy.
		retry := eff
		switch e.resolveCollision(path, plan.Final) {
		case CollisionOverwrite, CollisionOverwriteAll:
			retry.OnCollision = preset.CollisionOverwrite
		case CollisionRename, CollisionRenameAll:
			retry.OnCollision = preset.CollisionNumber
		default:
			retry.OnCollision = preset.CollisionSkip
		}
		if retry.OnCollision != preset.CollisionSkip {
			plan, err = output.PrepareWithSuffix(path, retry, suffix)
			if err != nil {
				res.Err = err
				res.Duration = time.Since(start)
				return res
			}
		}
		if plan.Skip {
			res.Skipped = true
			res.SkipReason = "output exists"
			res.Duration = time.Since(start)
			return res
		}
	}
	res.Output = plan.Final

	args = e.withThreads(args)
	res.Args = append([]cjxl.Arg(nil), args...)
	encodeStart := time.Now()
	res.StartedAt = encodeStart
	if e.OnFileStart != nil {
		e.OnFileStart(FileStarted{Input: path, PID: 0, StartedAt: encodeStart})
	}
	runRes := e.runEncode(ctx, args, path, plan.TempPath, func(pid int) {
		res.PID = pid
		if e.OnFileStart != nil {
			e.OnFileStart(FileStarted{Input: path, PID: pid, StartedAt: encodeStart})
		}
	})
	if res.PID == 0 {
		res.PID = runRes.PID
	}
	if !runRes.Success() {
		_ = os.Remove(plan.TempPath)
		res.Output = ""
		if ctx.Err() != nil {
			res.Cancelled = true
		} else {
			res.Err = encodeError(runRes)
		}
		res.Duration = time.Since(start)
		return res
	}

	finOpt := output.FinalizeOptions{Route: route, Verifier: e.deps.Verifier}
	if route == routes.RouteTranscode {
		finOpt.OriginalJPEG = path
	}
	if err := output.Finalize(ctx, plan, finOpt); err != nil {
		res.Output = ""
		if ctx.Err() != nil {
			res.Cancelled = true
		} else {
			res.Err = err
		}
		res.Duration = time.Since(start)
		return res
	}

	// Sidecar runs after a successful finalize: its failures only warn, and a
	// failed conversion never leaves a sidecar behind.
	if eff.JXLInfoSidecar {
		e.writeSidecar(ctx, &res, plan)
	}

	if fi, err := os.Stat(plan.Final); err == nil {
		res.OutputSize = fi.Size()
	}
	res.Duration = time.Since(start)
	return res
}

// writeSidecar inspects the finalized output and writes the jxlinfo sidecar
// next to it, always overwriting. Failures only warn: the conversion itself
// already succeeded.
func (e *Engine) writeSidecar(ctx context.Context, res *FileResult, plan output.Plan) {
	if e.deps.Inspector == nil {
		res.Warning = "jxlinfo sidecar requested but no inspector is configured"
		return
	}
	info, err := e.deps.Inspector.Inspect(ctx, plan.Final)
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled during teardown; the file already succeeded
		}
		res.Warning = err.Error()
		return
	}
	if err := os.WriteFile(output.SidecarPath(plan.Final), []byte(info+"\n"), 0o644); err != nil {
		res.Warning = "jxlinfo sidecar: " + err.Error()
	}
}

// resolveCollision decides an output-exists collision and reports the chosen
// action. Sticky answers (skip-all / overwrite-all / rename-all)
// short-circuit later collisions without calling the handler.
func (e *Engine) resolveCollision(input, target string) CollisionAction {
	e.mu.Lock()
	sticky := e.collisionAll
	cancelled := e.cancelled
	e.mu.Unlock()
	switch sticky {
	case CollisionSkipAll:
		return CollisionSkip
	case CollisionOverwriteAll:
		return CollisionOverwrite
	case CollisionRenameAll:
		return CollisionRename
	}
	if cancelled || e.CollisionHandler == nil {
		return CollisionSkip
	}
	action := e.CollisionHandler(input, target)
	if action == CollisionSkipAll || action == CollisionOverwriteAll || action == CollisionRenameAll {
		e.mu.Lock()
		e.collisionAll = action
		e.mu.Unlock()
	}
	return action
}

// startReporter is implemented by encoders that can report the OS PID.
type startReporter interface {
	RunWithStart(ctx context.Context, args []cjxl.Arg, input, output string, onStart func(pid int)) cjxl.Result
}

// runEncode runs one encode, using the PID-reporting path when the encoder
// supports it (the real *cjxl.Runner does; test fakes use plain Run).
func (e *Engine) runEncode(ctx context.Context, args []cjxl.Arg, input, output string, onStart func(pid int)) cjxl.Result {
	if sr, ok := e.deps.Encoder.(startReporter); ok {
		return sr.RunWithStart(ctx, args, input, output, onStart)
	}
	return e.deps.Encoder.Run(ctx, args, input, output)
}

// withThreads injects --num_threads when configured and not already set.
func (e *Engine) withThreads(args []cjxl.Arg) []cjxl.Arg {
	if e.settings.Threads <= 0 {
		return args
	}
	for _, a := range args {
		if a.Key == "--num_threads" {
			return args
		}
	}
	out := make([]cjxl.Arg, len(args), len(args)+1)
	copy(out, args)
	return append(out, cjxl.Arg{Key: "--num_threads", Value: strconv.Itoa(e.settings.Threads)})
}

// Progress returns a snapshot of the current state.
func (e *Engine) Progress() Progress {
	e.mu.Lock()
	remaining := e.bytesTotal - e.bytesDone
	p := Progress{
		Total:      e.total,
		Completed:  e.completed,
		Failed:     e.failed,
		Skipped:    e.skipped,
		InFlight:   e.inflight,
		BytesTotal: e.bytesTotal,
		BytesDone:  e.bytesDone,
		Coalesced:  e.coalesced,
		Paused:     e.paused,
	}
	e.mu.Unlock()
	p.Throughput = e.tp.rate()
	p.ETA = e.tp.eta(remaining)
	return p
}

func (e *Engine) emitProgress() {
	if e.OnProgress != nil {
		e.OnProgress(e.Progress())
	}
}
