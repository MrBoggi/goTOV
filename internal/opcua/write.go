package opcua

import "context"

// WriteNodeValue is kept for compatibility with existing callers. All writes
// go through WriteTag so validation, connection checks and retry behavior stay
// consistent.
func (c *Client) WriteNodeValue(ctx context.Context, nodeID string, value interface{}) error {
	return c.WriteTag(ctx, nodeID, value)
}
