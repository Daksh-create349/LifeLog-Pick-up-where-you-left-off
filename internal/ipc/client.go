package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lifelog/lifelog/internal/storage"
)

// Client is a typed JSON-RPC 2.0 client communicating over a Unix domain socket.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	nextID int64
}

// Dial connects to a LIFELOG daemon listening on a Unix domain socket.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dialing unix socket %s: %w", socketPath, err)
	}

	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Close closes the underlying socket connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// RawConn returns the underlying net.Conn for low-level protocol testing.
func (c *Client) RawConn() net.Conn {
	return c.conn
}

// Call sends a JSON-RPC 2.0 request and unmarshals the result into dest.
func (c *Client) Call(method string, params any, dest any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idVal := atomic.AddInt64(&c.nextID, 1)
	idRaw, _ := json.Marshal(idVal)

	var paramsRaw json.RawMessage
	if params != nil {
		pBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshaling params: %w", err)
		}
		paramsRaw = pBytes
	}

	req := Request{
		JSONRPC: ProtocolVersion,
		ID:      idRaw,
		Method:  method,
		Params:  paramsRaw,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	_ = c.conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.conn.Write(append(reqBytes, '\n')); err != nil {
		return fmt.Errorf("writing request: %w", err)
	}

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("unmarshaling response: %w (raw: %s)", err, string(line))
	}

	if resp.Error != nil {
		return resp.Error
	}

	if dest != nil && resp.Result != nil {
		resultBytes, err := json.Marshal(resp.Result)
		if err != nil {
			return fmt.Errorf("re-marshaling result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, dest); err != nil {
			return fmt.Errorf("unmarshaling into destination: %w", err)
		}
	}

	return nil
}

// Auth performs the authentication handshake.
func (c *Client) Auth(token string) (*AuthResult, error) {
	var result AuthResult
	err := c.Call("auth", AuthParams{Token: token}, &result)
	if err != nil {
		return nil, err
	}
	if !result.Authenticated {
		return nil, errors.New("authentication handshake rejected")
	}
	return &result, nil
}

// Health queries the daemon health report.
func (c *Client) Health() (*HealthResult, error) {
	var result HealthResult
	err := c.Call("health", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetProjects queries discovered projects with options.
func (c *Client) GetProjects(params GetProjectsParams) (*GetProjectsResult, error) {
	var result GetProjectsResult
	err := c.Call("getProjects", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTimeline queries activity timeline with filters.
func (c *Client) GetTimeline(params GetTimelineParams) (*GetTimelineResult, error) {
	var result GetTimelineResult
	err := c.Call("getTimeline", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetResumption queries resumption synthesis for a target project.
func (c *Client) GetResumption(params GetResumptionParams) (*GetResumptionResult, error) {
	var result GetResumptionResult
	err := c.Call("getResumption", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAppUsage queries aggregated app usage statistics.
func (c *Client) GetAppUsage(params GetAppUsageParams) (*GetAppUsageResult, error) {
	var result GetAppUsageResult
	err := c.Call("getAppUsage", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAppSessions queries individual application sessions.
func (c *Client) GetAppSessions(params GetAppSessionsParams) (*GetAppSessionsResult, error) {
	var result GetAppSessionsResult
	err := c.Call("getAppSessions", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTrackingStatus queries runtime status of all observers.
func (c *Client) GetTrackingStatus() (*TrackingStatusResult, error) {
	var result TrackingStatusResult
	err := c.Call("getTrackingStatus", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SetAppTracking toggles application usage tracking.
func (c *Client) SetAppTracking(params SetAppTrackingParams) (*SetAppTrackingResult, error) {
	var result SetAppTrackingResult
	err := c.Call("setAppTracking", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSystemEvents queries system activity transitions.
func (c *Client) GetSystemEvents(params GetSystemEventsParams) (*GetSystemEventsResult, error) {
	var result GetSystemEventsResult
	err := c.Call("getSystemEvents", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSystemActivitySummary queries aggregate system event counts.
func (c *Client) GetSystemActivitySummary(params GetSystemActivitySummaryParams) (*storage.SystemActivitySummary, error) {
	var result storage.SystemActivitySummary
	err := c.Call("getSystemActivitySummary", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetBrowserTrackingStatus queries browser tracking privacy and operational state.
func (c *Client) GetBrowserTrackingStatus() (*GetBrowserTrackingStatusResult, error) {
	var result GetBrowserTrackingStatusResult
	err := c.Call("getBrowserTrackingStatus", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SetBrowserTracking configures browser tracking state.
func (c *Client) SetBrowserTracking(params SetBrowserTrackingParams) (*SetBrowserTrackingResult, error) {
	var result SetBrowserTrackingResult
	err := c.Call("setBrowserTracking", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDataStats queries database file metrics and record counts.
func (c *Client) GetDataStats() (*GetDataStatsResult, error) {
	var result GetDataStatsResult
	err := c.Call("getDataStats", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRetentionPolicy queries data retention configuration.
func (c *Client) GetRetentionPolicy() (*GetRetentionPolicyResult, error) {
	var result GetRetentionPolicyResult
	err := c.Call("getRetentionPolicy", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SetRetentionPolicy updates data retention period in days.
func (c *Client) SetRetentionPolicy(params SetRetentionPolicyParams) (*SetRetentionPolicyResult, error) {
	var result SetRetentionPolicyResult
	err := c.Call("setRetentionPolicy", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RunRetentionCleanup triggers an immediate purge of expired records.
func (c *Client) RunRetentionCleanup() (*RunRetentionCleanupResult, error) {
	var result RunRetentionCleanupResult
	err := c.Call("runRetentionCleanup", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteActivity selectively purges activity records for a source.
func (c *Client) DeleteActivity(params DeleteActivityParams) (*DeleteActivityResult, error) {
	var result DeleteActivityResult
	err := c.Call("deleteActivity", params, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ExportUserData exports full local user data to structured JSON.
func (c *Client) ExportUserData() (*ExportUserDataResult, error) {
	var result ExportUserDataResult
	err := c.Call("exportUserData", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
