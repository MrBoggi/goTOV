package opcua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

// Client wraps the underlying gopcua.Client with logging and helper methods.
type Client struct {
	conn    *opcua.Client
	log     zerolog.Logger
	Updates chan TagUpdate // 👈 internal event channel for tag updates

	mu           sync.RWMutex
	displayNames map[string]string // 🔧 cache of NodeID → DisplayName
	connected    bool              // 👈 track connection status
}

// TagUpdate represents a single OPC UA value update
type TagUpdate struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Value       interface{} `json:"value"`
	Type        string      `json:"type"`
}

// NewClient creates an OPC UA client supporting both Anonymous and Username/Password authentication.
func NewClient(endpoint, username, password string, log zerolog.Logger) (*Client, error) {
	if strings.Contains(endpoint, "<ip_or_hostname>") {
		return nil, fmt.Errorf("OPC UA endpoint is not configured: found placeholder '<ip_or_hostname>'. Please update your configuration (e.g., config.yaml or GOTOV_OPCUA_ENDPOINT) with the correct server address")
	}
	// --- Discover endpoints with timeout ---
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoints, err := opcua.GetEndpoints(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("get endpoints: %w", err)
	}

	// --- Prefer SecurityPolicy=None for Beckhoff PLCs (simple setups) ---
	ep, err := opcua.SelectEndpoint(endpoints, "None", ua.MessageSecurityModeFromString("None"))
	if err != nil {
		return nil, fmt.Errorf("select endpoint: %w", err)
	}

	// --- Choose user token type based on credentials ---
	var userToken ua.UserTokenType
	if username != "" {
		userToken = ua.UserTokenTypeUserName
	} else {
		userToken = ua.UserTokenTypeAnonymous
	}

	// --- Build options ---
	opts := []opcua.Option{
		opcua.SecurityPolicy("None"),
		opcua.SecurityModeString("None"),
		opcua.SecurityMode(ua.MessageSecurityModeNone),
		opcua.SecurityFromEndpoint(ep, userToken),
		opcua.ApplicationName("goTOV"),
		opcua.ApplicationURI("urn:gotov:opcua:client"),
		opcua.ProductURI("urn:gotov:product"),
	}

	// --- Add authentication ---
	if username != "" {
		log.Info().Msgf("🔐 Using username authentication for OPC UA user '%s'", username)
		opts = append(opts, opcua.AuthUsername(username, password))
	} else {
		log.Info().Msg("🕶 Using anonymous authentication (no credentials)")
		opts = append(opts, opcua.AuthAnonymous())
	}

	// --- Create client ---
	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return &Client{
		conn:         c,
		log:          log,
		Updates:      make(chan TagUpdate, 100),
		displayNames: make(map[string]string),
	}, nil
}

// Connect establishes the OPC UA session with a timeout.
func (c *Client) Connect() error {
	c.log.Info().Msg("Connecting to OPC UA server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.conn.Connect(ctx); err != nil {
		c.setConnected(false)
		return fmt.Errorf("connect failed: %w", err)
	}

	c.setConnected(true)
	c.log.Info().Msg("✅ Connected to OPC UA server")
	return nil
}

// Close terminates the session gracefully.
func (c *Client) Close() error {
	c.log.Info().Msg("Closing OPC UA connection...")
	c.setConnected(false)
	return c.conn.Close(context.Background())
}

func (c *Client) setConnected(status bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = status
}

// IsConnected returns the current connection status
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// WaitForConnection blocks until the client is connected or the context is cancelled.
func (c *Client) WaitForConnection(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if c.IsConnected() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

// ReadNodeValue reads a single node value and returns the raw value (interface{}).
func (c *Client) ReadNodeValue(ctx context.Context, nodeID string) (interface{}, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("read failed: client is not connected")
	}

	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}

	req := &ua.ReadRequest{
		MaxAge: 2000,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id},
		},
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}

	var resp *ua.ReadResponse
	for i := 0; i < 3; i++ { // Retry up to 3 times for session errors
		resp, err = c.conn.Read(ctx, req)
		if err == nil {
			break
		}

		switch {
		case errors.Is(err, io.EOF) && c.conn.State() != opcua.Closed:
			time.Sleep(500 * time.Millisecond)
			continue
		case errors.Is(err, ua.StatusBadSessionIDInvalid),
			errors.Is(err, ua.StatusBadSessionNotActivated),
			errors.Is(err, ua.StatusBadSecureChannelIDInvalid),
			errors.Is(err, ua.StatusBadServerNotConnected):
			c.log.Warn().Err(err).Msgf("⚠️ Read retry %d/%d due to session/connection error", i+1, 3)
			time.Sleep(500 * time.Millisecond)
			continue
		default:
			return nil, fmt.Errorf("read failed: %w", err)
		}
	}

	if resp == nil || len(resp.Results) == 0 {
		return nil, fmt.Errorf("no response or empty results")
	}
	if resp.Results[0].Status != ua.StatusOK {
		return nil, fmt.Errorf("status not OK for node %s: %v", nodeID, resp.Results[0].Status)
	}

	val := resp.Results[0].Value.Value()
	if val == nil {
		return nil, fmt.Errorf("value is nil for node %s", nodeID)
	}

	return val, nil
}

// GetDisplayName retrieves the stored display name for a node, or the nodeID itself if not found.
func (c *Client) GetDisplayName(nodeID string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if name, ok := c.displayNames[nodeID]; ok {
		return name
	}
	return nodeID
}

// 🔧 Utility: store display names for later use
func (c *Client) SetDisplayName(nodeID, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.displayNames[nodeID] = name
}

// WriteTag writes a value to a specific node (tag) in the PLC.
func (c *Client) WriteTag(ctx context.Context, nodeID string, value interface{}) error {
	if !c.IsConnected() {
		return fmt.Errorf("write failed: client is not connected")
	}

	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return fmt.Errorf("invalid node id: %w", err)
	}

	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{
			{
				NodeID:      id,
				AttributeID: ua.AttributeIDValue,
				Value: &ua.DataValue{
					EncodingMask: ua.DataValueValue,
					Value:        ua.MustVariant(value),
				},
			},
		},
	}

	var resp *ua.WriteResponse
	for i := 0; i < 3; i++ { // Retry up to 3 times for session errors
		resp, err = c.conn.Write(ctx, req)
		if err == nil {
			break
		}

		switch {
		case errors.Is(err, ua.StatusBadSessionIDInvalid),
			errors.Is(err, ua.StatusBadSessionNotActivated),
			errors.Is(err, ua.StatusBadSecureChannelIDInvalid),
			errors.Is(err, ua.StatusBadServerNotConnected):
			c.log.Warn().Err(err).Msgf("⚠️ Write retry %d/%d due to session/connection error", i+1, 3)
			time.Sleep(500 * time.Millisecond)
			continue
		default:
			return fmt.Errorf("write failed: %w", err)
		}
	}

	if resp == nil || len(resp.Results) == 0 {
		return fmt.Errorf("no response or empty results from write")
	}

	if resp.Results[0] != ua.StatusOK {
		return fmt.Errorf("write response status not OK for node %s: %v", nodeID, resp.Results[0])
	}

	c.log.Info().Str("node", nodeID).Interface("value", value).Msg("✍️ Wrote to PLC tag")
	return nil
}
