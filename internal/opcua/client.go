package opcua

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

type Client struct {
	conn *opcua.Client
	log  zerolog.Logger
}

// NewClient creates a secure OPC UA client using certificate authentication.
// Falls back to anonymous if no credentials provided.
func NewClient(endpoint, username, password string, log zerolog.Logger) (*Client, error) {
	const (
		certFile = "certs/gotov_cert.pem"
		keyFile  = "certs/gotov_key.pem"
	)

	// Fetch endpoints from server to choose a compatible security policy.
	endpoints, err := opcua.GetEndpoints(context.Background(), endpoint)
	if err != nil {
		return nil, fmt.Errorf("get endpoints: %w", err)
	}

	// Prefer Basic256Sha256 + SignAndEncrypt if available.
	ep, err := opcua.SelectEndpoint(endpoints, ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSignAndEncrypt)
	if err != nil {
		log.Warn().Msg("No SignAndEncrypt endpoint found, falling back to Sign")
		ep, err = opcua.SelectEndpoint(endpoints, ua.SecurityPolicyURIBasic256Sha256, ua.MessageSecurityModeSign)
		if err != nil {
			return nil, fmt.Errorf("select endpoint: %w", err)
		}
	}

	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
		opcua.CertificateFile(certFile),
		opcua.PrivateKeyFile(keyFile),
		opcua.ApplicationName("goTOV"),
		opcua.ApplicationURI("urn:gotov:opcua:client"),
		opcua.ProductURI("urn:gotov:product"),
	}

	// Add username authentication if provided.
	if username != "" {
		opts = append(opts, opcua.AuthUsername(username, password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}
	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return &Client{conn: c, log: log}, nil
}

func (c *Client) Connect() error {
	c.log.Info().Msg("Connecting to OPC UA server...")
	if err := c.conn.Connect(context.Background()); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	c.log.Info().Msg("✅ Connected to OPC UA server")
	return nil
}

func (c *Client) Close() error {
	c.log.Info().Msg("Closing OPC UA connection...")
	return c.conn.Close(context.Background())
}

func (c *Client) ReadRaw(ctx context.Context, nodeID string) (*ua.ReadResponse, error) {
	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid NodeID: %w", err)
	}

	req := &ua.ReadRequest{
		MaxAge:             2000,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id, AttributeID: ua.AttributeIDValue},
		},
	}

	resp, err := c.conn.Read(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("read node %q failed: %w", nodeID, err)
	}
	return resp, nil
}
