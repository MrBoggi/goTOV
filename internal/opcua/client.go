package opcua

import (
	"context"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

type Client struct {
	conn *opcua.Client
	log  zerolog.Logger
}

func NewClient(endpoint, username, password string, log zerolog.Logger) (*Client, error) {
	opts := []opcua.Option{
		opcua.SecurityMode(ua.MessageSecurityModeNone),
		opcua.SecurityPolicy(ua.SecurityPolicyURINone),
	}
	opts = append(opts, opcua.AuthAnonymous())
	// Autentisering: Username eller Anonymous
	if username != "" {
		opts = append(opts, opcua.AuthUsername(username, password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}

	// ✅ Merk: v0.8.0 returnerer (*Client, error)
	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{conn: c, log: log}, nil
}

func (c *Client) Connect() error {
	return c.conn.Connect(context.Background())
}

func (c *Client) Close() error {
	return c.conn.Close(context.Background())
}

func (c *Client) ReadRaw(ctx context.Context, nodeID string) (*ua.ReadResponse, error) {
	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, err
	}

	req := &ua.ReadRequest{
		MaxAge:             2000,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{
				NodeID:      id,
				AttributeID: ua.AttributeIDValue,
			},
		},
	}

	return c.conn.Read(ctx, req)
}

// func (c *Client) ReadNodeValue(nodeID string) (interface{}, error) {
// 	id, err := ua.ParseNodeID(nodeID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req := &ua.ReadRequest{
// 		MaxAge:             2000,
// 		NodesToRead:        []*ua.ReadValueID{{NodeID: id, AttributeID: ua.AttributeIDValue}},
// 		TimestampsToReturn: ua.TimestampsToReturnBoth,
// 	}
// 	resp, err := c.conn.Read(context.Background(), req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if len(resp.Results) == 0 || resp.Results[0].Value == nil {
// 		return nil, nil
// 	}
// 	return resp.Results[0].Value.Value(), nil
// }
