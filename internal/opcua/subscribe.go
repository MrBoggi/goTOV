package opcua

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua" // 👈 nødvendig
)

// SubscribeAll subscribes to all provided nodes and logs value changes.
func (c *Client) SubscribeAll(ctx context.Context, nodes []*ua.NodeID) error {
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes to subscribe")
	}

	c.log.Info().Msgf("📡 Subscribing to %d nodes...", len(nodes))

	ch := make(chan *opcua.PublishNotificationData, 10)
	params := &opcua.SubscriptionParameters{Interval: time.Second}

	sub, err := c.conn.Subscribe(ctx, params, ch)
	if err != nil {
		return fmt.Errorf("create subscription failed: %w", err)
	}
	defer sub.Cancel(ctx)

	// --- Opprett handle→tag-navn map ---
	handleMap := make(map[uint32]string)

	// --- Opprett Monitored Items ---
	for i, id := range nodes {
		handle := uint32(i + 1000)
		handleMap[handle] = id.String()

		req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, handle)
		if _, err := sub.Monitor(ctx, ua.TimestampsToReturnBoth, req); err != nil {
			c.log.Warn().Err(err).Msgf("⚠️ Failed to monitor %v", id)
		}
	}

	// --- Lytte på meldinger ---
	go func() {
		for {
			select {
			case <-ctx.Done():
				c.log.Info().Msg("🛑 Subscription context cancelled")
				return
			case n := <-ch:
				if n == nil || n.Value == nil {
					continue
				}

				switch x := n.Value.(type) {
				case *ua.DataChangeNotification:
					for _, item := range x.MonitoredItems {
						if item.Value == nil || item.Value.Value == nil {
							continue
						}
						val := item.Value.Value.Value()
						tagName := handleMap[item.ClientHandle]

						// send update to channel
						c.Updates <- TagUpdate{
							Name:  tagName,
							Value: val,
							Type:  fmt.Sprintf("%T", val),
						}

						// and log to console
						c.log.Info().
							Msgf("🔄 %s = %v (%T)", tagName, val, val)
					}
				}
			}
		}
	}()

	c.log.Info().Msg("✅ Subscription started (Ctrl+C to stop)")
	<-ctx.Done()
	c.log.Info().Msg("🧭 Subscription stopped gracefully")
	return nil
}
