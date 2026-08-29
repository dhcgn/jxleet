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

// Deps are the external collaborators the engine needs.
type Deps struct {
	Encoder  Encoder
	Verifier output.Verifier // required for the replace policy; may be nil otherwise
}

// Settings configure a run. Processes and Threads are independent: Processes is
// how many cjxl invocations run in parallel, Threads is --num_threads passed to
// each (0 leaves it to the preset / cjxl default).
type Settings struct {
	Processes int
	Threads   int
	Preset    preset.Preset
	Deletion  output.DeletionByRoute
}

// FileResult is the outcome for one input file.
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
	Duration   time.Duration
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
	OnFile     func(FileResult)
	OnProgress func(Progress)

	mu       sync.Mutex
	cond     *sync.Cond
	pending  []string
	inflight int

	total      int
	completed  int
	failed     int
	skipped    int
	bytesTotal int64
	bytesDone  int64
	bytesOut   int64
	coalesced  int

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
	e := &Engine{deps: deps, settings: settings, tp: newThroughput(15)}
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
		res := e.process(e.ctx, path)
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
	plan, err := output.Prepare(path, eff)
	if err != nil {
		res.Err = err
		res.Duration = time.Since(start)
		return res
	}
	if plan.Skip {
		res.Skipped = true
		res.SkipReason = "output exists"
		res.Duration = time.Since(start)
		return res
	}
	res.Output = plan.Final

	args = e.withThreads(args)
	runRes := e.deps.Encoder.Run(ctx, args, path, plan.TempPath)
	if !runRes.Success() {
		os.Remove(plan.TempPath)
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

	if fi, err := os.Stat(plan.Final); err == nil {
		res.OutputSize = fi.Size()
	}
	res.Duration = time.Since(start)
	return res
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
