package opcua

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

// ProbeConnection performs a read-only standard server-status read. It checks
// the transport and session at the instant of the response, not PLC ownership,
// I/O state or suitability for a handover. ctx must bound the request.
func (c *Client) ProbeConnection(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("OPC UA client unavailable")
	}
	return probeServerState(ctx, c.conn.Read)
}
func probeServerState(ctx context.Context, read func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error)) error {
	req := &ua.ReadRequest{MaxAge: 0, NodesToRead: []*ua.ReadValueID{{NodeID: ua.NewNumericNodeID(0, id.Server_ServerStatus_State), AttributeID: ua.AttributeIDValue}}, TimestampsToReturn: ua.TimestampsToReturnNeither}
	resp, err := read(ctx, req)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if resp == nil || len(resp.Results) != 1 || resp.Results[0] == nil || resp.Results[0].Status != ua.StatusOK || resp.Results[0].Value == nil || resp.Results[0].Value.Value() == nil {
		return fmt.Errorf("invalid OPC UA server-state response")
	}
	switch state := resp.Results[0].Value.Value().(type) {
	case uint32:
		if state != uint32(ua.ServerStateRunning) {
			return fmt.Errorf("OPC UA server not running")
		}
	case int32:
		if state != int32(ua.ServerStateRunning) {
			return fmt.Errorf("OPC UA server not running")
		}
	case ua.ServerState:
		if state != ua.ServerStateRunning {
			return fmt.Errorf("OPC UA server not running")
		}
	default:
		return fmt.Errorf("invalid OPC UA server-state type")
	}
	return nil
}
