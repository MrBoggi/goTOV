package ads

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorStatus(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	disabled := NewMonitor(false, time.Minute).Status(now)
	require.Equal(t, "disabled", disabled.State)
	monitor := NewMonitor(true, time.Minute)
	require.Equal(t, "connecting", monitor.Status(now).State)
	monitor.Accepted(4, now)
	healthy := monitor.Status(now.Add(time.Second))
	require.Equal(t, "healthy", healthy.State)
	require.Equal(t, uint32(4), *healthy.LastSequence)
	monitor.Rejected("sequence_changed")
	degraded := monitor.Status(now.Add(2 * time.Minute))
	require.Equal(t, "degraded", degraded.State)
	require.Equal(t, "sequence_changed", *degraded.LastRejection)
}
