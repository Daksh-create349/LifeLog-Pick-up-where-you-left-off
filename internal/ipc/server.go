package ipc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lifelog/lifelog/internal/apps"
	"github.com/lifelog/lifelog/internal/storage"
)

const (
	// DefaultMaxPayloadBytes limits incoming request payload to 1 MiB to prevent memory exhaustion.
	DefaultMaxPayloadBytes int64 = 1 << 20

	// DefaultMaxConnections limits concurrent open client connections.
	DefaultMaxConnections = 32

	// DefaultRequestTimeout is the deadline for handling a single RPC call.
	DefaultRequestTimeout = 15 * time.Second

	// DefaultIdleTimeout is the read deadline between requests on an open connection.
	DefaultIdleTimeout = 120 * time.Second
)

// Config configures the IPC server.
type Config struct {
	SocketPath      string
	AuthToken       string
	DB              *storage.DB
	AppSupervisor   *apps.Supervisor
	Version         string
	MaxPayloadBytes int64
	MaxConnections  int
	RequestTimeout  time.Duration
	IdleTimeout     time.Duration
}

type session struct {
	id            string
	authenticated bool
	remoteAddr    string
	createdAt     time.Time
}

// Server is a local JSON-RPC 2.0 server operating over a Unix domain socket.
type Server struct {
	cfg         Config
	listener    net.Listener
	startTime   time.Time
	activeConns map[net.Conn]struct{}
	connsMu     sync.Mutex
	connSem     chan struct{}
	inFlight    sync.WaitGroup
	isClosing   atomic.Bool
	shutdownCh  chan struct{}
}

// NewServer initializes a new IPC Server and binds the Unix domain socket.
func NewServer(cfg Config) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, ErrSocketPathEmpty
	}
	if cfg.AuthToken == "" {
		return nil, errors.New("authentication token cannot be empty")
	}
	if cfg.DB == nil {
		return nil, errors.New("database instance cannot be nil")
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = DefaultMaxPayloadBytes
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = DefaultMaxConnections
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.Version == "" {
		cfg.Version = "0.1.0"
	}

	listener, err := Listen(cfg.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("binding ipc listener: %w", err)
	}

	return &Server{
		cfg:         cfg,
		listener:    listener,
		startTime:   time.Now().UTC(),
		activeConns: make(map[net.Conn]struct{}),
		connSem:     make(chan struct{}, cfg.MaxConnections),
		shutdownCh:  make(chan struct{}),
	}, nil
}

// Listener returns the active underlying net.Listener.
func (s *Server) Listener() net.Listener {
	return s.listener
}

// SocketPath returns the socket path configured for the server.
func (s *Server) SocketPath() string {
	return s.cfg.SocketPath
}

// Serve begins accepting client connections until context is cancelled or Shutdown is invoked.
func (s *Server) Serve(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			_ = s.Shutdown(context.Background())
		case <-s.shutdownCh:
		}
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isClosing.Load() {
				break
			}
			// Transient accept error
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return fmt.Errorf("accepting connection: %w", err)
		}

		// Enforce maximum concurrent connection limit
		select {
		case s.connSem <- struct{}{}:
			// Acquired connection slot
		default:
			// Server saturated: reject immediately with resource exceeded error
			resp := Response{
				JSONRPC: ProtocolVersion,
				ID:      json.RawMessage("null"),
				Error: &RPCError{
					Code:    CodeResourceExceeded,
					Message: fmt.Sprintf("connection limit reached (max %d)", s.cfg.MaxConnections),
				},
			}
			respBytes, _ := json.Marshal(resp)
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, _ = conn.Write(append(respBytes, '\n'))
			_ = conn.Close()
			continue
		}

		s.connsMu.Lock()
		s.activeConns[conn] = struct{}{}
		s.connsMu.Unlock()

		go s.handleConn(conn)
	}

	wg.Wait()
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		s.connsMu.Lock()
		delete(s.activeConns, conn)
		s.connsMu.Unlock()
		<-s.connSem
	}()

	sessID := generateSessionID()
	sess := &session{
		id:            sessID,
		authenticated: false,
		remoteAddr:    conn.RemoteAddr().String(),
		createdAt:     time.Now().UTC(),
	}

	reader := bufio.NewReader(conn)

	for {
		if s.isClosing.Load() {
			break
		}

		// Set idle timeout between requests
		_ = conn.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout))

		// Read newline-delimited message up to MaxPayloadBytes
		lineBytes, err := readBoundedLine(reader, s.cfg.MaxPayloadBytes)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Idle timeout reached
				break
			}
			if errors.Is(err, errLineTooLong) {
				errResp := Response{
					JSONRPC: ProtocolVersion,
					ID:      json.RawMessage("null"),
					Error: &RPCError{
						Code:    CodeResourceExceeded,
						Message: fmt.Sprintf("payload exceeds maximum allowed size of %d bytes", s.cfg.MaxPayloadBytes),
					},
				}
				s.sendResponse(conn, &errResp)
			}
			break
		}

		lineBytes = bytes.TrimSpace(lineBytes)
		if len(lineBytes) == 0 {
			continue
		}

		s.inFlight.Add(1)
		resp := s.dispatch(sess, lineBytes)
		s.inFlight.Done()

		if resp != nil {
			if !s.sendResponse(conn, resp) {
				break
			}
		}
	}
}

func (s *Server) sendResponse(conn net.Conn, resp *Response) bool {
	data, err := json.Marshal(resp)
	if err != nil {
		return false
	}
	_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.RequestTimeout))
	_, err = conn.Write(append(data, '\n'))
	return err == nil
}

func (s *Server) dispatch(sess *session, raw []byte) *Response {
	if s.isClosing.Load() {
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      json.RawMessage("null"),
			Error: &RPCError{
				Code:    CodeServerShuttingDown,
				Message: "server is shutting down",
			},
		}
	}

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      json.RawMessage("null"),
			Error: &RPCError{
				Code:    CodeParseError,
				Message: fmt.Sprintf("parse error: %v", err),
			},
		}
	}

	reqID := req.ID
	if len(reqID) == 0 {
		reqID = json.RawMessage("null")
	}

	if req.JSONRPC != ProtocolVersion {
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      reqID,
			Error: &RPCError{
				Code:    CodeInvalidRequest,
				Message: fmt.Sprintf("unsupported jsonrpc version %q (expected %q)", req.JSONRPC, ProtocolVersion),
			},
		}
	}

	if req.Method == "" {
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      reqID,
			Error: &RPCError{
				Code:    CodeInvalidRequest,
				Message: "method is required",
			},
		}
	}

	// Handshake authentication routing
	if req.Method == "auth" {
		result, rpcErr := handleAuth(sess, s, req.Params)
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      reqID,
			Result:  result,
			Error:   rpcErr,
		}
	}

	// Security: Reject any application query on unauthenticated sessions
	if !sess.authenticated {
		return &Response{
			JSONRPC: ProtocolVersion,
			ID:      reqID,
			Error: &RPCError{
				Code:    CodeUnauthorized,
				Message: "unauthorized: session not authenticated; call 'auth' first",
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()

	var result any
	var rpcErr *RPCError

	switch req.Method {
	case "health":
		result, rpcErr = handleHealth(ctx, s)
	case "getProjects":
		result, rpcErr = handleGetProjects(ctx, s, req.Params)
	case "getTimeline":
		result, rpcErr = handleGetTimeline(ctx, s, req.Params)
	case "getResumption":
		result, rpcErr = handleGetResumption(ctx, s, req.Params)
	case "getAppUsage":
		result, rpcErr = handleGetAppUsage(ctx, s, req.Params)
	case "getAppSessions":
		result, rpcErr = handleGetAppSessions(ctx, s, req.Params)
	case "getTrackingStatus":
		result, rpcErr = handleGetTrackingStatus(ctx, s)
	case "setAppTracking":
		result, rpcErr = handleSetAppTracking(ctx, s, req.Params)
	case "getSystemEvents":
		result, rpcErr = handleGetSystemEvents(ctx, s, req.Params)
	case "getSystemActivitySummary":
		result, rpcErr = handleGetSystemActivitySummary(ctx, s, req.Params)
	case "getBrowserTrackingStatus":
		result, rpcErr = handleGetBrowserTrackingStatus(ctx, s)
	case "setBrowserTracking":
		result, rpcErr = handleSetBrowserTracking(ctx, s, req.Params)
	case "getDataStats":
		result, rpcErr = handleGetDataStats(ctx, s)
	case "getRetentionPolicy":
		result, rpcErr = handleGetRetentionPolicy(ctx, s)
	case "setRetentionPolicy":
		result, rpcErr = handleSetRetentionPolicy(ctx, s, req.Params)
	case "runRetentionCleanup":
		result, rpcErr = handleRunRetentionCleanup(ctx, s)
	case "deleteActivity":
		result, rpcErr = handleDeleteActivity(ctx, s, req.Params)
	case "exportUserData":
		result, rpcErr = handleExportUserData(ctx, s)
	default:
		rpcErr = &RPCError{
			Code:    CodeMethodNotFound,
			Message: fmt.Sprintf("method not found: %s", req.Method),
		}
	}

	return &Response{
		JSONRPC: ProtocolVersion,
		ID:      reqID,
		Result:  result,
		Error:   rpcErr,
	}
}

// Shutdown gracefully terminates the server, closes listeners, drains in-flight requests,
// and safely removes the socket file.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.isClosing.Swap(true) {
		return nil
	}
	close(s.shutdownCh)

	// 1. Close listener to reject new connections
	if s.listener != nil {
		_ = s.listener.Close()
	}

	// 2. Close active client connections
	s.connsMu.Lock()
	for conn := range s.activeConns {
		_ = conn.Close()
	}
	s.connsMu.Unlock()

	// 3. Wait for in-flight requests to complete or context to expire
	doneCh := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-ctx.Done():
	}

	// 4. Safely clean up socket file (only if it is a socket)
	cleanPath := s.cfg.SocketPath
	if info, err := os.Lstat(cleanPath); err == nil && info.Mode()&os.ModeSocket != 0 {
		_ = os.Remove(cleanPath)
	}

	return nil
}

var errLineTooLong = errors.New("line too long")

func readBoundedLine(r *bufio.Reader, maxBytes int64) ([]byte, error) {
	var buf bytes.Buffer
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		if int64(buf.Len()+len(chunk)) > maxBytes {
			return nil, errLineTooLong
		}
		buf.Write(chunk)
		if !isPrefix {
			break
		}
	}
	return buf.Bytes(), nil
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}

// GenerateAuthToken generates a cryptographically secure 256-bit random authentication token.
func GenerateAuthToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating cryptographically secure random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
