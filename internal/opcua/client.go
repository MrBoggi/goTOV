package opcua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

// Client wraps the underlying gopcua.Client with logging and helper methods.
type Client struct {
	conn         *opcua.Client
	log          zerolog.Logger
	Updates      chan TagUpdate    // 👈 internal event channel for tag updates
	displayNames map[string]string // 🔧 cache of NodeID → DisplayName
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
		displayNames: make(map[string]string), // 🔧 initialize here
	}, nil
}

// Connect establishes the OPC UA session with a timeout.
func (c *Client) Connect() error {
	c.log.Info().Msg("Connecting to OPC UA server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.conn.Connect(ctx); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	c.log.Info().Msg("✅ Connected to OPC UA server")
	return nil
}

// Close terminates the session gracefully.
func (c *Client) Close() error {
	c.log.Info().Msg("Closing OPC UA connection...")
	return c.conn.Close(context.Background())
}

// ReadNodeValue reads a single node value and returns the raw value (interface{}).
func (c *Client) ReadNodeValue(ctx context.Context, nodeID string) (interface{}, error) {
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
	for {
		resp, err = c.conn.Read(ctx, req)
		if err == nil {
			break
		}

		switch {
		case errors.Is(err, io.EOF) && c.conn.State() != opcua.Closed:
			time.Sleep(time.Second)
			continue
		case errors.Is(err, ua.StatusBadSessionIDInvalid),
			errors.Is(err, ua.StatusBadSessionNotActivated),
			errors.Is(err, ua.StatusBadSecureChannelIDInvalid):
			time.Sleep(time.Second)
			continue
		default:
			return nil, fmt.Errorf("read failed: %w", err)
		}
	}

	if resp == nil || len(resp.Results) == 0 {
		return nil, fmt.Errorf("no response or empty results")
	}
	if resp.Results[0].Status != ua.StatusOK {
		return nil, fmt.Errorf("status not OK: %v", resp.Results[0].Status)
	}

	val := resp.Results[0].Value.Value()
	if val == nil {
		return nil, fmt.Errorf("value is nil")
	}

	return val, nil
}

// GetDisplayName retrieves the stored display name for a node, or the nodeID itself if not found.
func (c *Client) GetDisplayName(nodeID string) string {
	if name, ok := c.displayNames[nodeID]; ok {
		return name
	}
	return nodeID
}

// 🔧 Utility: store display names for later use
func (c *Client) SetDisplayName(nodeID, name string) {
	c.displayNames[nodeID] = name
}

// WriteTag writes a value to a specific node (tag) in the PLC.
func (c *Client) WriteTag(ctx context.Context, nodeID string, value interface{}) error {
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

	resp, err := c.conn.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if resp.Results[0] != ua.StatusOK {
		return fmt.Errorf("write response status not OK: %v", resp.Results[0])
	}

	c.log.Info().Str("node", nodeID).Interface("value", value).Msg("✍️ Wrote to PLC tag")
	return nil
}
