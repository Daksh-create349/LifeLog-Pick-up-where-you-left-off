package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/apps"
	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/ipc"
	"github.com/lifelog/lifelog/internal/storage"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ll_*")
	if err != nil {
		t.Fatalf("failed to create short temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func setupTestServer(t *testing.T) (*ipc.Server, string, *storage.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	sockPath := filepath.Join(shortTempDir(t), "s.sock")
	token := "test_secret_token_1234567890abcdef"

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	server, err := ipc.NewServer(ipc.Config{
		SocketPath:     sockPath,
		AuthToken:      token,
		DB:             db,
		Version:        "0.1.0-test",
		RequestTimeout: 5 * time.Second,
		IdleTimeout:    10 * time.Second,
	})
	if err != nil {
		db.Close()
		t.Fatalf("failed to create ipc server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer sCancel()
		_ = server.Shutdown(shutdownCtx)
		_ = db.Close()
	})

	// Wait for listener to be active
	time.Sleep(50 * time.Millisecond)

	return server, sockPath, db, token
}

func TestIPC_AuthLifecycle_SuccessAndFailure(t *testing.T) {
	_, sockPath, _, token := setupTestServer(t)

	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("failed to dial socket: %v", err)
	}
	defer client.Close()

	// 1. Calling health before auth must fail with CodeUnauthorized (-32001)
	_, err = client.Health()
	if err == nil {
		t.Fatalf("expected health check before auth to fail, got success")
	}
	if !isRPCErrorCode(err, ipc.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized (-32001), got: %v", err)
	}

	// 2. Auth with empty token must fail
	_, err = client.Auth("")
	if err == nil || !isRPCErrorCode(err, ipc.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized on empty token, got: %v", err)
	}

	// 3. Auth with incorrect token must fail
	_, err = client.Auth("wrong-token-abc")
	if err == nil || !isRPCErrorCode(err, ipc.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized on incorrect token, got: %v", err)
	}

	// 4. Auth with correct token must succeed
	authRes, err := client.Auth(token)
	if err != nil {
		t.Fatalf("auth failed with valid token: %v", err)
	}
	if !authRes.Authenticated {
		t.Fatalf("expected Authenticated: true")
	}
	if authRes.SessionID == "" {
		t.Fatalf("expected non-empty SessionID")
	}

	// 5. Calling health after successful auth must succeed
	healthRes, err := client.Health()
	if err != nil {
		t.Fatalf("health check failed after auth: %v", err)
	}
	if healthRes.ProtocolVersion != ipc.ProtocolVersion {
		t.Errorf("expected protocol version %s, got %s", ipc.ProtocolVersion, healthRes.ProtocolVersion)
	}
	if healthRes.DaemonStatus != "healthy" {
		t.Errorf("expected daemon status 'healthy', got %s", healthRes.DaemonStatus)
	}
	if healthRes.SchemaVersion < 7 {
		t.Errorf("expected schema version >= 7, got %d", healthRes.SchemaVersion)
	}
}

func TestIPC_InvalidAndMalformedRequests(t *testing.T) {
	_, sockPath, _, token := setupTestServer(t)

	// Raw connection tests
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// 1. Send completely malformed JSON
	_, _ = conn.Write([]byte("{invalid-json-payload\n"))
	var resp ipc.Response
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response for malformed JSON: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ipc.CodeParseError {
		t.Fatalf("expected ParseError (-32700), got: %+v", resp.Error)
	}

	// 2. Send unsupported jsonrpc version
	badVerReq := []byte(`{"jsonrpc":"1.0","id":1,"method":"auth","params":{"token":"` + token + `"}}` + "\n")
	_, _ = conn.Write(badVerReq)
	resp = ipc.Response{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response for bad version: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidRequest {
		t.Fatalf("expected InvalidRequest (-32600), got: %+v", resp.Error)
	}

	// 3. Send request without method
	noMethodReq := []byte(`{"jsonrpc":"2.0","id":2,"params":{"token":"` + token + `"}}` + "\n")
	_, _ = conn.Write(noMethodReq)
	resp = ipc.Response{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response for missing method: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidRequest {
		t.Fatalf("expected InvalidRequest (-32600), got: %+v", resp.Error)
	}

	// 4. Authenticate properly, then send unknown method
	authReq := []byte(`{"jsonrpc":"2.0","id":3,"method":"auth","params":{"token":"` + token + `"}}` + "\n")
	_, _ = conn.Write(authReq)
	resp = ipc.Response{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode auth response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("auth failed: %v", resp.Error)
	}

	unknownReq := []byte(`{"jsonrpc":"2.0","id":4,"method":"nonExistentMethod"}` + "\n")
	_, _ = conn.Write(unknownReq)
	resp = ipc.Response{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode unknown method response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ipc.CodeMethodNotFound {
		t.Fatalf("expected MethodNotFound (-32601), got: %+v", resp.Error)
	}
}

func TestIPC_MalformedParameters(t *testing.T) {
	_, sockPath, _, token := setupTestServer(t)

	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("failed to dial socket: %v", err)
	}
	defer client.Close()

	if _, err := client.Auth(token); err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	// 1. getProjects with invalid sort
	_, err = client.GetProjects(ipc.GetProjectsParams{Sort: "bad_sort_mode"})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on invalid sort mode, got: %v", err)
	}

	// 2. getProjects with invalid limit
	_, err = client.GetProjects(ipc.GetProjectsParams{Limit: 9999})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on excessive limit, got: %v", err)
	}

	// 3. getTimeline with invalid domain
	_, err = client.GetTimeline(ipc.GetTimelineParams{Domain: "unknown_domain"})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on invalid domain, got: %v", err)
	}

	// 4. getTimeline with negative offset
	_, err = client.GetTimeline(ipc.GetTimelineParams{Offset: -5})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on negative offset, got: %v", err)
	}

	// 5. getResumption with empty project_path
	_, err = client.GetResumption(ipc.GetResumptionParams{ProjectPath: ""})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on empty project_path, got: %v", err)
	}

	// 6. getResumption with invalid gap duration
	_, err = client.GetResumption(ipc.GetResumptionParams{ProjectPath: ".", Gap: "invalid-duration"})
	if !isRPCErrorCode(err, ipc.CodeInvalidParams) {
		t.Errorf("expected CodeInvalidParams on invalid gap, got: %v", err)
	}
}

func TestIPC_OversizedRequestRejection(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	sockPath := filepath.Join(shortTempDir(t), "s.sock")
	token := "token_secret_12345"

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Server configured with small 512-byte payload limit
	server, err := ipc.NewServer(ipc.Config{
		SocketPath:      sockPath,
		AuthToken:       token,
		DB:              db,
		MaxPayloadBytes: 512,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Shutdown(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send 2048 bytes (exceeds 512 byte limit)
	oversized := make([]byte, 2048)
	for i := range oversized {
		oversized[i] = 'a'
	}
	oversized[len(oversized)-1] = '\n'

	_, _ = conn.Write(oversized)

	var resp ipc.Response
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response for oversized request: %v", err)
	}

	if resp.Error == nil || resp.Error.Code != ipc.CodeResourceExceeded {
		t.Fatalf("expected CodeResourceExceeded (-32002), got: %+v", resp.Error)
	}
}

func TestIPC_SocketPermissionsAndStaleSocketSafety(t *testing.T) {
	tempDir := t.TempDir()
	sockDir := shortTempDir(t)
	sockPath := filepath.Join(sockDir, "perm.sock")
	dbPath := filepath.Join(tempDir, "perm.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	server, err := ipc.NewServer(ipc.Config{
		SocketPath: sockPath,
		AuthToken:  "token",
		DB:         db,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	// 1. Verify socket file permissions (0600 on POSIX)
	info, err := os.Lstat(sockPath)
	if err != nil {
		t.Fatalf("lstat socket: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("expected file to be a socket, mode: %v", info.Mode())
	}
	perm := info.Mode().Perm()
	if perm&0777 != 0600 {
		t.Errorf("expected socket permissions 0600, got %04o", perm)
	}

	// 2. Active daemon conflict check: PrepareSocket should fail because server is active
	err = ipc.PrepareSocket(sockPath)
	if !errors.Is(err, ipc.ErrDaemonAlreadyRunning) {
		t.Errorf("expected ErrDaemonAlreadyRunning, got: %v", err)
	}

	// Shut down server
	_ = server.Shutdown(context.Background())

	// 3. Stale socket check: create a dead socket
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("creating dead socket: %v", err)
	}
	_ = listener.Close() // closed, but file remains on disk

	// PrepareSocket should successfully detect that no active process is listening and clean it up
	if err := ipc.PrepareSocket(sockPath); err != nil {
		t.Fatalf("expected PrepareSocket to clean stale socket, got: %v", err)
	}
	if _, err := os.Lstat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale socket to be deleted from disk")
	}

	// 4. Safety: Refuse to delete regular files
	regFile := filepath.Join(sockDir, "regular.txt")
	if err := os.WriteFile(regFile, []byte("important user data"), 0644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	err = ipc.PrepareSocket(regFile)
	if !errors.Is(err, ipc.ErrSocketNotASocket) {
		t.Errorf("expected ErrSocketNotASocket on regular file, got: %v", err)
	}
	// Verify data file was not deleted
	if _, err := os.Stat(regFile); os.IsNotExist(err) {
		t.Fatalf("regular file was deleted! Safe handling violated.")
	}

	// 5. Safety: Refuse to delete symlinks
	symPath := filepath.Join(sockDir, "symlink.sock")
	if err := os.Symlink(regFile, symPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	err = ipc.PrepareSocket(symPath)
	if !errors.Is(err, ipc.ErrSocketIsSymlink) {
		t.Errorf("expected ErrSocketIsSymlink on symlink, got: %v", err)
	}
}

func TestIPC_GracefulShutdownAndSocketCleanup(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := filepath.Join(shortTempDir(t), "shutdown.sock")
	dbPath := filepath.Join(tempDir, "shutdown.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	server, err := ipc.NewServer(ipc.Config{
		SocketPath: sockPath,
		AuthToken:  "secret",
		DB:         db,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Verify socket exists while running
	if _, err := os.Lstat(sockPath); err != nil {
		t.Fatalf("socket does not exist: %v", err)
	}

	// Shut down server
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Verify socket file has been deleted
	if _, err := os.Lstat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("expected socket file to be deleted on shutdown, err: %v", err)
	}

	// Verify client dial fails after shutdown
	if _, err := ipc.Dial(sockPath); err == nil {
		t.Fatalf("expected dial to fail after shutdown, got success")
	}
}

func TestIPC_ConcurrentRequestsAndRaceSafety(t *testing.T) {
	_, sockPath, db, token := setupTestServer(t)

	// Seed some filesystem events for queries
	now := time.Now().UTC()
	err := db.RecordEvents(context.Background(), []events.Event{
		{
			ID:        "evt_001",
			Type:      events.EventCreated,
			Path:      "/test/project/file.go",
			Size:      123,
			ModTime:   now,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("record events: %v", err)
	}

	const numClients = 10
	const requestsPerClient = 15

	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			client, err := ipc.Dial(sockPath)
			if err != nil {
				t.Errorf("client %d dial failed: %v", clientID, err)
				return
			}
			defer client.Close()

			if _, err := client.Auth(token); err != nil {
				t.Errorf("client %d auth failed: %v", clientID, err)
				return
			}

			for j := 0; j < requestsPerClient; j++ {
				// Rotate between health, getProjects, and getTimeline
				switch j % 3 {
				case 0:
					h, err := client.Health()
					if err != nil || h.DaemonStatus != "healthy" {
						t.Errorf("client %d health call %d failed: %v", clientID, j, err)
					}
				case 1:
					p, err := client.GetProjects(ipc.GetProjectsParams{Limit: 10})
					if err != nil {
						t.Errorf("client %d getProjects call %d failed: %v", clientID, j, err)
					}
					_ = p
				case 2:
					tl, err := client.GetTimeline(ipc.GetTimelineParams{Limit: 5})
					if err != nil {
						t.Errorf("client %d getTimeline call %d failed: %v", clientID, j, err)
					}
					_ = tl
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestIPC_ReadMethods_DataFidelity(t *testing.T) {
	_, sockPath, db, token := setupTestServer(t)

	// Record sample events
	now := time.Now().UTC()
	projDir := filepath.Join(t.TempDir(), "active_project")
	_ = os.MkdirAll(projDir, 0755)

	err := db.RecordEvents(context.Background(), []events.Event{
		{
			ID:        "evt_fidelity_1",
			Type:      events.EventCreated,
			Path:      filepath.Join(projDir, "main.go"),
			Size:      500,
			ModTime:   now,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("record events: %v", err)
	}

	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := client.Auth(token); err != nil {
		t.Fatalf("auth: %v", err)
	}

	// 1. Test getProjects
	projectsRes, err := client.GetProjects(ipc.GetProjectsParams{Limit: 50})
	if err != nil {
		t.Fatalf("getProjects failed: %v", err)
	}
	if len(projectsRes.Projects) == 0 {
		t.Fatalf("expected at least 1 project in discovery")
	}

	// 2. Test getTimeline
	timelineRes, err := client.GetTimeline(ipc.GetTimelineParams{Domain: "filesystem", Limit: 10})
	if err != nil {
		t.Fatalf("getTimeline failed: %v", err)
	}
	if len(timelineRes.Items) == 0 {
		t.Fatalf("expected at least 1 timeline item")
	}
	if timelineRes.Items[0].Domain != "filesystem" {
		t.Errorf("expected domain 'filesystem', got %s", timelineRes.Items[0].Domain)
	}

	// 3. Test getResumption
	resumeRes, err := client.GetResumption(ipc.GetResumptionParams{ProjectPath: projDir, Limit: 10})
	if err != nil {
		t.Fatalf("getResumption failed: %v", err)
	}
	if resumeRes.Report == nil {
		t.Fatalf("expected non-nil ResumptionReport")
	}
	if resumeRes.Report.Observed.Filesystem.TotalEvents == 0 {
		t.Errorf("expected at least 1 filesystem event in resumption report")
	}
}

func TestIPC_AppMethods_DataFidelity(t *testing.T) {
	tempDir := shortTempDir(t)
	dbPath := filepath.Join(tempDir, "ipc_apps.db")
	sockPath := filepath.Join(tempDir, "ipc_apps.sock")
	token := "ipc_apps_secret_token_12345"

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Seed 2 app sessions in db
	sess1 := &storage.AppSession{
		ID:          "app_sess_test_01",
		AppBundleID: "com.apple.Terminal",
		AppName:     "Terminal",
		StartTime:   now.Add(-20 * time.Minute),
		EndTime:     now.Add(-10 * time.Minute),
		DurationMs:  600000,
		IsActive:    false,
		CreatedAt:   now.Add(-20 * time.Minute),
	}
	sess2 := &storage.AppSession{
		ID:          "app_sess_test_02",
		AppBundleID: "com.microsoft.VSCode",
		AppName:     "Code",
		StartTime:   now.Add(-10 * time.Minute),
		EndTime:     now,
		DurationMs:  600000,
		IsActive:    false,
		CreatedAt:   now.Add(-10 * time.Minute),
	}
	if err := db.RecordAppSession(ctx, sess1); err != nil {
		t.Fatalf("seeding sess1: %v", err)
	}
	if err := db.RecordAppSession(ctx, sess2); err != nil {
		t.Fatalf("seeding sess2: %v", err)
	}

	appSup, err := apps.NewSupervisor(apps.SupervisorConfig{
		DB:      db,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("NewSupervisor failed: %v", err)
	}

	server, err := ipc.NewServer(ipc.Config{
		SocketPath:    sockPath,
		AuthToken:     token,
		DB:            db,
		AppSupervisor: appSup,
		Version:       "0.2.0",
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	go func() {
		_ = server.Serve(context.Background())
	}()

	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	// Authenticate first
	if _, err := client.Auth(token); err != nil {
		t.Fatalf("Auth failed: %v", err)
	}

	// 1. Test getTrackingStatus
	trStatus, err := client.GetTrackingStatus()
	if err != nil {
		t.Fatalf("GetTrackingStatus failed: %v", err)
	}
	if !trStatus.AppTrackingEnabled {
		t.Errorf("expected AppTrackingEnabled true")
	}

	// 2. Test getAppUsage
	usage, err := client.GetAppUsage(ipc.GetAppUsageParams{Window: "1h"})
	if err != nil {
		t.Fatalf("GetAppUsage failed: %v", err)
	}
	if usage.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", usage.TotalSessions)
	}
	if usage.TotalDurationMs != 1200000 {
		t.Errorf("expected 1200000 ms, got %d", usage.TotalDurationMs)
	}
	if len(usage.Apps) != 2 {
		t.Errorf("expected 2 apps in usage result, got %d", len(usage.Apps))
	}

	// 3. Test getAppSessions
	sessionsRes, err := client.GetAppSessions(ipc.GetAppSessionsParams{Limit: 10})
	if err != nil {
		t.Fatalf("GetAppSessions failed: %v", err)
	}
	if sessionsRes.TotalCount != 2 || len(sessionsRes.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got total=%d len=%d", sessionsRes.TotalCount, len(sessionsRes.Sessions))
	}

	// 4. Test setAppTracking toggle
	setRes, err := client.SetAppTracking(ipc.SetAppTrackingParams{Enabled: false})
	if err != nil {
		t.Fatalf("SetAppTracking(false) failed: %v", err)
	}
	if setRes.Status.AppTrackingEnabled {
		t.Errorf("expected AppTrackingEnabled false after disabling")
	}
}

func TestIPC_SocketPathTooLong(t *testing.T) {
	// 105 character long socket path
	longPath := "/tmp/this_is_a_very_long_path_that_exceeds_the_darwin_unix_domain_socket_sun_path_104_byte_buffer_limit.sock"
	err := ipc.PrepareSocket(longPath)
	if !errors.Is(err, ipc.ErrSocketPathTooLong) {
		t.Errorf("expected ErrSocketPathTooLong, got: %v", err)
	}
}

func isRPCErrorCode(err error, expectedCode int) bool {
	if err == nil {
		return false
	}
	var rpcErr *ipc.RPCError
	if ok := isRPCError(err, &rpcErr); ok && rpcErr.Code == expectedCode {
		return true
	}
	// Check string format as fallback
	return false
}

func isRPCError(err error, target **ipc.RPCError) bool {
	var e *ipc.RPCError
	if ok := errorAsRPC(err, &e); ok {
		*target = e
		return true
	}
	return false
}

func errorAsRPC(err error, target **ipc.RPCError) bool {
	if err == nil {
		return false
	}
	if r, ok := err.(*ipc.RPCError); ok {
		*target = r
		return true
	}
	return false
}

func TestIPC_SystemAndDataManagementMethods(t *testing.T) {
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "ipc_sys.sock")
	dbPath := filepath.Join(tempDir, "ipc_sys.db")
	token := "secure_test_token_sys_123456789012"
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	// Insert dummy system events
	_ = db.RecordSystemEvent(ctx, &storage.SystemEvent{
		ID:        "sys_ipc_1",
		EventType: storage.SystemEventSleep,
		Timestamp: now.Add(-10 * time.Minute),
	})
	_ = db.RecordSystemEvent(ctx, &storage.SystemEvent{
		ID:        "sys_ipc_2",
		EventType: storage.SystemEventWake,
		Timestamp: now.Add(-5 * time.Minute),
	})

	server, err := ipc.NewServer(ipc.Config{
		SocketPath: sockPath,
		AuthToken:  token,
		DB:         db,
		Version:    "1.0.0-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = server.Serve(context.Background())
	}()

	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	if _, err := client.Auth(token); err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	// 1. Test getSystemEvents
	eventsRes, err := client.GetSystemEvents(ipc.GetSystemEventsParams{Limit: 10})
	if err != nil {
		t.Fatalf("GetSystemEvents failed: %v", err)
	}
	if eventsRes.Count != 2 {
		t.Errorf("expected 2 system events, got %d", eventsRes.Count)
	}

	// 2. Test getSystemActivitySummary
	sumRes, err := client.GetSystemActivitySummary(ipc.GetSystemActivitySummaryParams{})
	if err != nil {
		t.Fatalf("GetSystemActivitySummary failed: %v", err)
	}
	if sumRes.TotalEvents != 2 || sumRes.SleepCount != 1 || sumRes.WakeCount != 1 {
		t.Errorf("unexpected system activity summary: %+v", sumRes)
	}

	// 3. Test getBrowserTrackingStatus and setBrowserTracking
	bStatus, err := client.GetBrowserTrackingStatus()
	if err != nil {
		t.Fatalf("GetBrowserTrackingStatus failed: %v", err)
	}
	if bStatus.Enabled {
		t.Errorf("expected browser tracking to be disabled by default")
	}

	bSet, err := client.SetBrowserTracking(ipc.SetBrowserTrackingParams{Enabled: true})
	if err != nil {
		t.Fatalf("SetBrowserTracking failed: %v", err)
	}
	if !bSet.Status.Enabled {
		t.Errorf("expected browser tracking preference updated to true")
	}

	// 4. Test getDataStats
	statsRes, err := client.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats failed: %v", err)
	}
	if statsRes.Stats.SystemEventsCount != 2 {
		t.Errorf("expected 2 system events in stats, got %d", statsRes.Stats.SystemEventsCount)
	}

	// 5. Test getRetentionPolicy and setRetentionPolicy
	retRes, err := client.GetRetentionPolicy()
	if err != nil {
		t.Fatalf("GetRetentionPolicy failed: %v", err)
	}
	if retRes.RetentionDays != 30 {
		t.Errorf("expected default 30 days retention, got %d", retRes.RetentionDays)
	}

	setRetRes, err := client.SetRetentionPolicy(ipc.SetRetentionPolicyParams{RetentionDays: 60})
	if err != nil {
		t.Fatalf("SetRetentionPolicy failed: %v", err)
	}
	if !setRetRes.Success || setRetRes.RetentionDays != 60 {
		t.Errorf("unexpected set retention result: %+v", setRetRes)
	}

	// 6. Test runRetentionCleanup
	cleanupRes, err := client.RunRetentionCleanup()
	if err != nil {
		t.Fatalf("RunRetentionCleanup failed: %v", err)
	}
	if cleanupRes.PurgeStats == nil {
		t.Errorf("expected purge stats returned")
	}

	// 7. Test exportUserData
	exportRes, err := client.ExportUserData()
	if err != nil {
		t.Fatalf("ExportUserData failed: %v", err)
	}
	if exportRes.Export == nil || len(exportRes.Export.SystemEvents) != 2 {
		t.Errorf("expected 2 system events in export, got %+v", exportRes.Export)
	}

	// 8. Test deleteActivity
	delRes, err := client.DeleteActivity(ipc.DeleteActivityParams{Source: "system"})
	if err != nil {
		t.Fatalf("DeleteActivity failed: %v", err)
	}
	if !delRes.Success || delRes.DeletedCount != 2 {
		t.Errorf("expected 2 deleted system records, got %+v", delRes)
	}

	// Verify post-deletion stats
	statsAfter, err := client.GetDataStats()
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.Stats.SystemEventsCount != 0 {
		t.Errorf("expected 0 system events after delete, got %d", statsAfter.Stats.SystemEventsCount)
	}
}
