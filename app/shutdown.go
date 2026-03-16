// Package app provides graceful shutdown management for VPN Manager.
// This module implements proper cleanup of resources, goroutines, and
// connections when the application is terminating.
//
// Following best practices from Kubernetes, systemd, and POSIX signals.
package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ShutdownTimeout is the default maximum time to wait for graceful shutdown.
const ShutdownTimeout = 30 * time.Second

// ShutdownPhase represents different phases of the shutdown process.
type ShutdownPhase int

const (
	// PhaseRunning indicates normal operation.
	PhaseRunning ShutdownPhase = iota
	// PhaseShuttingDown indicates shutdown has been initiated.
	PhaseShuttingDown
	// PhaseGracePeriod indicates waiting for operations to complete.
	PhaseGracePeriod
	// PhaseForcedShutdown indicates forced termination.
	PhaseForcedShutdown
	// PhaseCompleted indicates shutdown is complete.
	PhaseCompleted
)

func (p ShutdownPhase) String() string {
	switch p {
	case PhaseRunning:
		return "running"
	case PhaseShuttingDown:
		return "shutting_down"
	case PhaseGracePeriod:
		return "grace_period"
	case PhaseForcedShutdown:
		return "forced_shutdown"
	case PhaseCompleted:
		return "completed"
	default:
		return "unknown"
	}
}

// ShutdownPriority determines the order of cleanup operations.
type ShutdownPriority int

const (
	// PriorityFirst runs before everything else (e.g., stop accepting new connections).
	PriorityFirst ShutdownPriority = 100
	// PriorityHigh runs early (e.g., notify users, save state).
	PriorityHigh ShutdownPriority = 200
	// PriorityNormal is the default priority (e.g., disconnect VPN).
	PriorityNormal ShutdownPriority = 500
	// PriorityLow runs late (e.g., restore network settings).
	PriorityLow ShutdownPriority = 800
	// PriorityLast runs at the very end (e.g., close logs).
	PriorityLast ShutdownPriority = 900
)

// ShutdownHook is a function that runs during shutdown.
type ShutdownHook struct {
	Name     string
	Priority ShutdownPriority
	Timeout  time.Duration
	Hook     func(context.Context) error
}

// ShutdownManager coordinates graceful shutdown of all components.
type ShutdownManager struct {
	mu sync.RWMutex

	phase        ShutdownPhase
	hooks        []ShutdownHook
	timeout      time.Duration
	cancelFunc   context.CancelFunc
	shutdownOnce sync.Once

	// Active goroutine tracking
	wg        sync.WaitGroup
	activeOps int64

	// Channels for coordination
	shutdownCh  chan struct{}
	completedCh chan struct{}

	// Callbacks
	onPhaseChange func(ShutdownPhase)
}

// Global shutdown manager instance
var (
	globalShutdownManager     *ShutdownManager
	globalShutdownManagerOnce sync.Once
)

// GetShutdownManager returns the global shutdown manager.
func GetShutdownManager() *ShutdownManager {
	globalShutdownManagerOnce.Do(func() {
		globalShutdownManager = NewShutdownManager(ShutdownTimeout)
	})
	return globalShutdownManager
}

// NewShutdownManager creates a new shutdown manager.
func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	return &ShutdownManager{
		phase:       PhaseRunning,
		hooks:       make([]ShutdownHook, 0),
		timeout:     timeout,
		shutdownCh:  make(chan struct{}),
		completedCh: make(chan struct{}),
	}
}

// SetTimeout sets the shutdown timeout.
func (sm *ShutdownManager) SetTimeout(timeout time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.timeout = timeout
}

// SetOnPhaseChange sets a callback for phase changes.
func (sm *ShutdownManager) SetOnPhaseChange(callback func(ShutdownPhase)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onPhaseChange = callback
}

// RegisterHook adds a shutdown hook.
func (sm *ShutdownManager) RegisterHook(hook ShutdownHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if hook.Timeout == 0 {
		hook.Timeout = 10 * time.Second
	}

	sm.hooks = append(sm.hooks, hook)

	// Sort by priority
	for i := len(sm.hooks) - 1; i > 0; i-- {
		if sm.hooks[i].Priority < sm.hooks[i-1].Priority {
			sm.hooks[i], sm.hooks[i-1] = sm.hooks[i-1], sm.hooks[i]
		}
	}
}

// Register is a convenience method for registering simple hooks.
func (sm *ShutdownManager) Register(name string, priority ShutdownPriority, hook func(context.Context) error) {
	sm.RegisterHook(ShutdownHook{
		Name:     name,
		Priority: priority,
		Timeout:  10 * time.Second,
		Hook:     hook,
	})
}

// TrackOperation marks the start of an operation that should complete before shutdown.
// Returns a done function that must be called when the operation completes.
func (sm *ShutdownManager) TrackOperation() func() {
	atomic.AddInt64(&sm.activeOps, 1)
	sm.wg.Add(1)

	return func() {
		atomic.AddInt64(&sm.activeOps, -1)
		sm.wg.Done()
	}
}

// ActiveOperations returns the number of tracked active operations.
func (sm *ShutdownManager) ActiveOperations() int64 {
	return atomic.LoadInt64(&sm.activeOps)
}

// IsShuttingDown returns true if shutdown has been initiated.
func (sm *ShutdownManager) IsShuttingDown() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.phase != PhaseRunning
}

// Phase returns the current shutdown phase.
func (sm *ShutdownManager) Phase() ShutdownPhase {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.phase
}

// ShutdownChan returns a channel that closes when shutdown begins.
func (sm *ShutdownManager) ShutdownChan() <-chan struct{} {
	return sm.shutdownCh
}

// CompletedChan returns a channel that closes when shutdown completes.
func (sm *ShutdownManager) CompletedChan() <-chan struct{} {
	return sm.completedCh
}

// setPhase updates the shutdown phase and notifies listeners.
func (sm *ShutdownManager) setPhase(phase ShutdownPhase) {
	sm.mu.Lock()
	sm.phase = phase
	callback := sm.onPhaseChange
	sm.mu.Unlock()

	if callback != nil {
		callback(phase)
	}

	// Emit event
	Emit(EventShutdown, "ShutdownManager", phase)
}

// Shutdown initiates graceful shutdown.
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	var err error

	sm.shutdownOnce.Do(func() {
		err = sm.doShutdown(ctx)
	})

	return err
}

func (sm *ShutdownManager) doShutdown(ctx context.Context) error {
	LogInfo("Shutdown: Initiating graceful shutdown")
	close(sm.shutdownCh)
	sm.setPhase(PhaseShuttingDown)

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, sm.timeout)
	defer cancel()
	sm.cancelFunc = cancel

	// Run hooks in priority order
	sm.mu.RLock()
	hooks := make([]ShutdownHook, len(sm.hooks))
	copy(hooks, sm.hooks)
	sm.mu.RUnlock()

	var errors ErrorList

	for _, hook := range hooks {
		LogInfo("Shutdown: Running hook '%s' (priority %d)", hook.Name, hook.Priority)

		hookCtx, hookCancel := context.WithTimeout(timeoutCtx, hook.Timeout)

		errCh := make(chan error, 1)
		go func(h ShutdownHook) {
			errCh <- h.Hook(hookCtx)
		}(hook)

		select {
		case hookErr := <-errCh:
			if hookErr != nil {
				LogError("Shutdown: Hook '%s' failed: %v", hook.Name, hookErr)
				errors.Add(hookErr)
			} else {
				LogInfo("Shutdown: Hook '%s' completed", hook.Name)
			}
		case <-hookCtx.Done():
			LogWarn("Shutdown: Hook '%s' timed out", hook.Name)
		}

		hookCancel()

		// Check if overall context is done
		select {
		case <-timeoutCtx.Done():
			LogWarn("Shutdown: Overall timeout reached, forcing shutdown")
			sm.setPhase(PhaseForcedShutdown)
			goto done
		default:
		}
	}

	// Wait for tracked operations
	sm.setPhase(PhaseGracePeriod)
	LogInfo("Shutdown: Waiting for %d active operations to complete", sm.ActiveOperations())

done:
	waitCh := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		LogInfo("Shutdown: All operations completed")
	case <-timeoutCtx.Done():
		LogWarn("Shutdown: Timeout waiting for operations, %d still active", sm.ActiveOperations())
	}

	sm.setPhase(PhaseCompleted)
	close(sm.completedCh)

	LogInfo("Shutdown: Complete")
	return errors.Combined()
}

// ForceShutdown immediately terminates without waiting.
func (sm *ShutdownManager) ForceShutdown() {
	LogWarn("Shutdown: Force shutdown initiated")
	sm.setPhase(PhaseForcedShutdown)

	if sm.cancelFunc != nil {
		sm.cancelFunc()
	}

	close(sm.completedCh)
}

// ═══════════════════════════════════════════════════════════════════════════
// SIGNAL HANDLING
// ═══════════════════════════════════════════════════════════════════════════

// InstallSignalHandlers sets up OS signal handlers for graceful shutdown.
// Handles SIGINT, SIGTERM, and SIGHUP.
func InstallSignalHandlers() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-sigCh
		LogInfo("Received signal: %v", sig)

		// Start graceful shutdown
		go GetShutdownManager().Shutdown(ctx)

		// Wait for second signal for force quit
		select {
		case sig = <-sigCh:
			LogWarn("Received second signal (%v), forcing shutdown", sig)
			GetShutdownManager().ForceShutdown()
			cancel()
			os.Exit(1)
		case <-GetShutdownManager().CompletedChan():
			cancel()
		}
	}()

	return ctx
}

// ═══════════════════════════════════════════════════════════════════════════
// GRACEFUL GOROUTINE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// Worker represents a managed goroutine that respects shutdown signals.
type Worker struct {
	name    string
	work    func(context.Context)
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	running atomic.Bool
}

// NewWorker creates a new managed worker.
func NewWorker(name string, work func(context.Context)) *Worker {
	return &Worker{
		name: name,
		work: work,
		done: make(chan struct{}),
	}
}

// Start begins the worker goroutine.
func (w *Worker) Start(parentCtx context.Context) {
	if w.running.Swap(true) {
		return // Already running
	}

	w.ctx, w.cancel = context.WithCancel(parentCtx)
	w.done = make(chan struct{})

	go func() {
		defer close(w.done)
		defer w.running.Store(false)

		LogDebug("Worker '%s' started", w.name)

		w.work(w.ctx)

		LogDebug("Worker '%s' stopped", w.name)
	}()
}

// Stop signals the worker to stop and waits for it.
func (w *Worker) Stop(timeout time.Duration) error {
	if !w.running.Load() {
		return nil
	}

	w.cancel()

	select {
	case <-w.done:
		return nil
	case <-time.After(timeout):
		return NewVPNError(ErrCodeProcessFailed, "worker stop timeout: "+w.name)
	}
}

// IsRunning returns whether the worker is running.
func (w *Worker) IsRunning() bool {
	return w.running.Load()
}

// Done returns a channel that closes when the worker stops.
func (w *Worker) Done() <-chan struct{} {
	return w.done
}

// WorkerPool manages a group of workers.
type WorkerPool struct {
	mu      sync.Mutex
	workers []*Worker
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(ctx context.Context) *WorkerPool {
	poolCtx, cancel := context.WithCancel(ctx)
	return &WorkerPool{
		workers: make([]*Worker, 0),
		ctx:     poolCtx,
		cancel:  cancel,
	}
}

// Add adds a worker to the pool and starts it.
func (wp *WorkerPool) Add(worker *Worker) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	wp.workers = append(wp.workers, worker)
	worker.Start(wp.ctx)
}

// Submit creates and starts a worker with the given function.
func (wp *WorkerPool) Submit(name string, work func(context.Context)) *Worker {
	worker := NewWorker(name, work)
	wp.Add(worker)
	return worker
}

// Stop stops all workers with the given timeout.
func (wp *WorkerPool) Stop(timeout time.Duration) error {
	wp.mu.Lock()
	workers := make([]*Worker, len(wp.workers))
	copy(workers, wp.workers)
	wp.mu.Unlock()

	wp.cancel()

	var errors ErrorList
	for _, w := range workers {
		if err := w.Stop(timeout); err != nil {
			errors.Add(err)
		}
	}

	return errors.Combined()
}

// Running returns the number of running workers.
func (wp *WorkerPool) Running() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	count := 0
	for _, w := range wp.workers {
		if w.IsRunning() {
			count++
		}
	}
	return count
}
