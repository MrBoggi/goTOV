package opcua

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua/ua"
)

func (c *Client) WriteNodeValue(ctx context.Context, nodeID string, value interface{}) error {
	id, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return fmt.Errorf("invalid node id: %w", err)
	}

	v, err := ua.NewVariant(value)
	if err != nil {
		return fmt.Errorf("invalid value: %w", err)
	}

	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID:      id,
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        v,
			},
		}},
	}

	resp, err := c.conn.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	if resp.Results[0] != ua.StatusOK {
		return fmt.Errorf("write not OK: %v", resp.Results[0])
	}

	c.log.Info().Msgf("✅ Wrote %v to %s", value, nodeID)
	return nil
}
