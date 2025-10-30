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
	if username != "" {
		opts = append(opts, opcua.AuthUsername(username, password))
	}

	c := opcua.NewClient(endpoint, opts...)
	return &Client{conn: c, log: log}, nil
}

func (c *Client) Connect() error {
	return c.conn.Connect(context.Background())
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) ReadNodeValue(nodeID string) (interface{}, error) {
	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	req := &ua.ReadRequest{
		MaxAge:             2000,
		NodesToRead:        []*ua.ReadValueID{{NodeID: id, AttributeID: ua.AttributeIDValue}},
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}
	resp, err := c.conn.Read(context.Background(), req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0].Value == nil {
		return nil, nil
	}
	return resp.Results[0].Value.Value(), nil
}
