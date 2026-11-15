package ads

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/stretchr/testify/require"
)

// TestInputsIntegration is deliberately opt-in because it opens a read-only
// ADS connection to a PLC. It never changes routes or writes process data.
// Symbol lookup uses administrative ReadWrite; Close releases handles with
// administrative ADS Write (0xF006).
func TestInputsIntegration(t *testing.T) {
	if os.Getenv("GOTOV_ADS_INTEGRATION") != "1" {
		t.Skip("set GOTOV_ADS_INTEGRATION=1 to run against a configured PLC")
	}

	cfg := config.ADSConfig{
		Enabled:        true,
		Host:           os.Getenv("GOTOV_ADS_HOST"),
		TargetNetID:    os.Getenv("GOTOV_ADS_TARGET_NET_ID"),
		LocalNetID:     os.Getenv("GOTOV_ADS_LOCAL_NET_ID"),
		Port:           48898,
		TargetPort:     851,
		LocalPort:      10500,
		RequestTimeout: "8s",
		PollInterval:   "5s",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := NewClient(cfg)
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.ReadState(ctx))
	require.NoError(t, client.LookupInputs(ctx))
	t.Logf("ADS symbol metadata: %+v", client.InputsMetadata())
	samples, err := CollectDiagnostic(ctx, client, 3, time.Second)
	require.NoError(t, err)
	for _, sample := range samples {
		t.Logf("Validated ADS sample: %+v", sample)
	}
}
