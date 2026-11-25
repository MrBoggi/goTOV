package opcua

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

func TestProbeReadsOnlyStandardServerStatus(t *testing.T) {
	v, err := ua.NewVariant(uint32(0))
	if err != nil {
		t.Fatal(err)
	}
	err = probeServerState(context.Background(), func(_ context.Context, req *ua.ReadRequest) (*ua.ReadResponse, error) {
		if len(req.NodesToRead) != 1 || req.NodesToRead[0].NodeID.String() != ua.NewNumericNodeID(0, id.Server_ServerStatus_State).String() || req.NodesToRead[0].AttributeID != ua.AttributeIDValue || req.MaxAge != 0 {
			t.Errorf("unexpected read request %+v", req)
		}
		return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: v}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestProbeRejectsTransportAndInvalidResponses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		resp *ua.ReadResponse
		err  error
	}{{context.Background(), nil, errors.New("down")}, {context.Background(), nil, nil}, {context.Background(), &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusBadNodeIDUnknown}}}, nil}, {ctx, &ua.ReadResponse{}, nil}} {
		if err := probeServerState(tc.ctx, func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) { return tc.resp, tc.err }); err == nil {
			t.Fatal("accepted failed probe")
		}
	}
}
func TestProbeHonorsRequestContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := probeServerState(ctx, func(ctx context.Context, _ *ua.ReadRequest) (*ua.ReadResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestProbeRejectsNonRunningAndBadType(t *testing.T) {
	for _, value := range []any{uint32(1), int32(4), "running"} {
		v, err := ua.NewVariant(value)
		if err != nil {
			t.Fatal(err)
		}
		err = probeServerState(context.Background(), func(context.Context, *ua.ReadRequest) (*ua.ReadResponse, error) {
			return &ua.ReadResponse{Results: []*ua.DataValue{{Status: ua.StatusOK, Value: v}}}, nil
		})
		if err == nil {
			t.Fatalf("accepted %v", value)
		}
	}
}
