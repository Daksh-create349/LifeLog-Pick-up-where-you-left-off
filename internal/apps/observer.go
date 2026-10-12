package apps

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/lifelog/lifelog/internal/storage"
)

const (
	ExpectedProtocolVersion = "1.0"
	MaxHelperLineLength     = 64 * 1024 // 64 KiB max line
	MaxCrashCount           = 3
	CrashWindow             = 60 * time.Second
	DefaultFallbackCap      = 30 * time.Second
)

// SupervisorConfig defines configuration parameters for the application supervisor.
type SupervisorConfig struct {
	DB             *storage.DB
	BinaryPath     string
	Enabled        bool
	FallbackCap    time.Duration
	CommandFactory func(ctx context.Context, binaryPath string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error)
}

// Supervisor manages the lifecycle of the native macOS application observer process.
type Supervisor struct {
	db             *storage.DB
	binaryPath     string
	enabled        bool
	running        bool
	status         string // "active", "paused", "crashed", "unavailable"
	fallbackCap    time.Duration
	commandFactory func(ctx context.Context, binaryPath string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error)

	currentSession   *storage.AppSession
	sessionMonoStart time.Time

	mu         sync.RWMutex
	cancelFunc context.CancelFunc
	workerWg   sync.WaitGroup
	crashTimes []time.Time
}

// NewSupervisor initializes an app observer supervisor.
func NewSupervisor(cfg SupervisorConfig) (*Supervisor, error) {
	if cfg.DB == nil {
		return nil, errors.New("database instance cannot be nil")
	}

	capDur := cfg.FallbackCap
	if capDur <= 0 {
		capDur = DefaultFallbackCap
	}

	binPath := cfg.BinaryPath
	if binPath == "" {
		binPath = locateOrBuildHelper()
	}

	cmdFactory := cfg.CommandFactory
	if cmdFactory == nil {
		cmdFactory = defaultCommandFactory
	}

	s := &Supervisor{
		db:             cfg.DB,
		binaryPath:     binPath,
		enabled:        cfg.Enabled,
		status:         "paused",
		fallbackCap:    capDur,
		commandFactory: cmdFactory,
	}

	if cfg.Enabled {
		s.status = "active"
	}

	return s, nil
}

func defaultCommandFactory(ctx context.Context, binaryPath string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	if binaryPath == "" {
		return nil, nil, nil, errors.New("helper binary not found")
	}

	cmd := exec.CommandContext(ctx, binaryPath)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return nil, nil, nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	return cmd, stdinPipe, stdoutPipe, nil
}

// Start begins supervising the app observer.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// 1. Reconcile any open sessions from previous crashes
	if _, err := s.db.CloseOpenAppSessions(ctx, s.fallbackCap); err != nil {
		// Log warning but continue
		_ = err
	}

	s.running = true
	if !s.enabled {
		s.status = "paused"
		return nil
	}

	if s.binaryPath == "" {
		s.status = "unavailable"
		return nil
	}

	s.status = "active"
	subCtx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	s.workerWg.Add(1)
	go s.supervisorLoop(subCtx)

	return nil
}

// Stop cleanly terminates the observer and finalizes the active session.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}

	s.running = false
	s.status = "paused"
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	s.mu.Unlock()

	s.workerWg.Wait()

	// Finalize current session if open
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalizeCurrentSessionLocked()

	return nil
}

// SetEnabled enables or disables tracking at runtime.
func (s *Supervisor) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.enabled == enabled {
		return nil
	}
	s.enabled = enabled

	if !enabled {
		s.status = "paused"
		if s.cancelFunc != nil {
			s.cancelFunc()
			s.cancelFunc = nil
		}
		s.finalizeCurrentSessionLocked()
		return nil
	}

	// Enabled
	if s.binaryPath == "" {
		s.status = "unavailable"
		return nil
	}

	s.status = "active"
	if s.running && s.cancelFunc == nil {
		subCtx, cancel := context.WithCancel(context.Background())
		s.cancelFunc = cancel
		s.workerWg.Add(1)
		go s.supervisorLoop(subCtx)
	}

	return nil
}

// Status returns current tracking metrics and state.
func (s *Supervisor) Status() TrackingStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := TrackingStatus{
		Enabled: s.enabled,
		Running: s.running && s.status == "active",
		Status:  s.status,
	}

	if s.currentSession != nil {
		st.ActiveApp = s.currentSession.AppName
		st.ActiveBundle = s.currentSession.AppBundleID
		st.ActiveSessionID = s.currentSession.ID
		t := s.currentSession.StartTime
		st.ActiveSince = &t
		if !s.sessionMonoStart.IsZero() {
			st.ActiveDurationMs = time.Since(s.sessionMonoStart).Milliseconds()
		}
	}

	return st
}

func (s *Supervisor) supervisorLoop(ctx context.Context) {
	defer s.workerWg.Done()

	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.mu.RLock()
		enabled := s.enabled
		bin := s.binaryPath
		s.mu.RUnlock()

		if !enabled || bin == "" {
			return
		}

		// Check crash limits within sliding window
		now := time.Now()
		s.mu.Lock()
		validCrashes := make([]time.Time, 0, len(s.crashTimes))
		for _, t := range s.crashTimes {
			if now.Sub(t) <= CrashWindow {
				validCrashes = append(validCrashes, t)
			}
		}
		s.crashTimes = validCrashes

		if len(s.crashTimes) >= MaxCrashCount {
			s.status = "crashed"
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		// Run child process
		err := s.runChildProcess(ctx)
		if ctx.Err() != nil {
			return
		}

		// Process terminated
		s.mu.Lock()
		s.finalizeCurrentSessionLocked()
		s.crashTimes = append(s.crashTimes, time.Now())
		s.status = "recovering"
		s.mu.Unlock()

		_ = err
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			if backoff < 8*time.Second {
				backoff *= 2
			}
		}

		s.mu.Lock()
		if s.enabled {
			s.status = "active"
		}
		s.mu.Unlock()
	}
}

func (s *Supervisor) runChildProcess(ctx context.Context) error {
	cmd, stdinPipe, stdoutPipe, err := s.commandFactory(ctx, s.binaryPath)
	if err != nil {
		return err
	}
	defer stdinPipe.Close()
	defer stdoutPipe.Close()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting helper process: %w", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	buf := make([]byte, MaxHelperLineLength)
	scanner.Buffer(buf, MaxHelperLineLength)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			s.handleMessageBytes(line)
		}
	}()

	select {
	case <-ctx.Done():
		// Clean teardown: close stdin so child helper exits immediately
		_ = stdinPipe.Close()
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
		return nil
	case <-readDone:
		// Child terminated or stdout closed
		return cmd.Wait()
	}
}

func (s *Supervisor) handleMessageBytes(raw []byte) {
	var msg HelperMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	if msg.Version != ExpectedProtocolVersion {
		return
	}

	ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, msg.Timestamp)
		if err != nil {
			ts = time.Now().UTC()
		}
	}
	ts = ts.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled {
		return
	}

	switch msg.Event {
	case EventAppActivated:
		s.handleAppActivatedLocked(msg.BundleID, msg.AppName, ts)
	case EventSystemSleep:
		s.handleSystemSleepLocked(ts)
	case EventSystemWake:
		s.handleSystemWakeLocked(ts)
	case EventSystemLock:
		s.handleSystemLockLocked(ts)
	case EventSystemUnlock:
		s.handleSystemUnlockLocked(ts)
	case EventHeartbeat:
		s.handleHeartbeatLocked(ts)
	}
}

func (s *Supervisor) handleAppActivatedLocked(bundleID, appName string, ts time.Time) {
	if bundleID == "" && appName == "" {
		return
	}
	if appName == "" {
		appName = bundleID
	}

	// 1. Deduplicate: if same app is already active, ignore
	if s.currentSession != nil && s.currentSession.AppBundleID == bundleID && s.currentSession.AppName == appName {
		return
	}

	// 2. Finalize previous session
	s.finalizeCurrentSessionLocked()

	// 3. Open new session
	sessID := generateSessionID()
	newSess := &storage.AppSession{
		ID:          sessID,
		AppBundleID: bundleID,
		AppName:     appName,
		StartTime:   ts,
		EndTime:     ts,
		DurationMs:  0,
		IsActive:    true,
		CreatedAt:   ts,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.db.RecordAppSession(ctx, newSess); err == nil {
		s.currentSession = newSess
		s.sessionMonoStart = time.Now()
	}
}

func (s *Supervisor) handleSystemSleepLocked(ts time.Time) {
	s.finalizeCurrentSessionLocked()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.db.RecordSystemEvent(ctx, &storage.SystemEvent{
		EventType: storage.SystemEventSleep,
		Timestamp: ts,
	})
}

func (s *Supervisor) handleSystemWakeLocked(ts time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.db.RecordSystemEvent(ctx, &storage.SystemEvent{
		EventType: storage.SystemEventWake,
		Timestamp: ts,
	})
}

func (s *Supervisor) handleSystemLockLocked(ts time.Time) {
	s.finalizeCurrentSessionLocked()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.db.RecordSystemEvent(ctx, &storage.SystemEvent{
		EventType: storage.SystemEventLock,
		Timestamp: ts,
	})
}

func (s *Supervisor) handleSystemUnlockLocked(ts time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.db.RecordSystemEvent(ctx, &storage.SystemEvent{
		EventType: storage.SystemEventUnlock,
		Timestamp: ts,
	})
}

func (s *Supervisor) handleHeartbeatLocked(ts time.Time) {
	if s.currentSession == nil {
		return
	}

	var durMs int64
	if !s.sessionMonoStart.IsZero() {
		durMs = time.Since(s.sessionMonoStart).Milliseconds()
	} else {
		durMs = ts.Sub(s.currentSession.StartTime).Milliseconds()
	}
	if durMs < 0 {
		durMs = 0
	}

	s.currentSession.EndTime = ts
	s.currentSession.DurationMs = durMs

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = s.db.UpdateAppSession(ctx, s.currentSession.ID, ts, durMs, true)
}

func (s *Supervisor) finalizeCurrentSessionLocked() {
	if s.currentSession == nil {
		return
	}

	now := time.Now().UTC()
	var durMs int64
	if !s.sessionMonoStart.IsZero() {
		durMs = time.Since(s.sessionMonoStart).Milliseconds()
	} else {
		durMs = now.Sub(s.currentSession.StartTime).Milliseconds()
	}
	if durMs < 0 {
		durMs = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = s.db.UpdateAppSession(ctx, s.currentSession.ID, now, durMs, false)
	s.currentSession = nil
	s.sessionMonoStart = time.Time{}
}

func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("app_sess_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

func locateOrBuildHelper() string {
	// 1. Relative bin/lifelog-app-observer
	cwd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(cwd, "bin", "lifelog-app-observer"),
		filepath.Join(cwd, "..", "bin", "lifelog-app-observer"),
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "lifelog-app-observer"))
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "bin", "lifelog-app-observer"))
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}

	// 2. Try building if swift source exists and swiftc is in PATH
	swiftSrc := filepath.Join(cwd, "cmd", "app-observer", "main.swift")
	if info, err := os.Stat(swiftSrc); err == nil && !info.IsDir() {
		outBin := filepath.Join(cwd, "bin", "lifelog-app-observer")
		_ = os.MkdirAll(filepath.Dir(outBin), 0755)
		cmd := exec.Command("swiftc", "-O", "-o", outBin, swiftSrc)
		if err := cmd.Run(); err == nil {
			return outBin
		}
	}

	return ""
}
