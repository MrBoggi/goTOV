package opcua

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// SubscribeAll subscribes to all provided nodes and pushes updates to c.Updates.
func (c *Client) SubscribeAll(ctx context.Context, nodes []*ua.NodeID) error {
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes to subscribe")
	}

	c.log.Info().Msgf("📡 Subscribing to %d nodes...", len(nodes))

	ch := make(chan *opcua.PublishNotificationData, 20)
	params := &opcua.SubscriptionParameters{Interval: time.Second}

	sub, err := c.conn.Subscribe(ctx, params, ch)
	if err != nil {
		return fmt.Errorf("create subscription failed: %w", err)
	}

	// --- Opprett handle → tag-navn map ---
	handleMap := make(map[uint32]string)
	for i, id := range nodes {
		handle := uint32(i + 1000)
		handleMap[handle] = id.String()

		req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, handle)
		if _, err := sub.Monitor(ctx, ua.TimestampsToReturnBoth, req); err != nil {
			c.log.Warn().Err(err).Msgf("⚠️ Failed to monitor %v", id)
		}
	}

	// --- Les meldinger kontinuerlig ---
	go func() {
		defer func() {
			// avslutt sub når context stoppes eller loop avsluttes
			_ = sub.Cancel(context.Background())
			c.log.Info().Msg("🧭 Subscription stopped gracefully")
		}()

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
						tag := handleMap[item.ClientHandle]

						display := ""
						if c.displayNames != nil {
							display = c.displayNames[tag]
						}

						// Logg til konsoll
						c.log.Info().Msgf("🔄 %s = %v (%T)", display, val, val)

						// Push til WS via kanal
						select {
						case c.Updates <- TagUpdate{
							Name:        tag,
							DisplayName: display, // 👈 bruker variabelen
							Value:       val,
							Type:        fmt.Sprintf("%T", val),
						}:
						default:
							c.log.Warn().Msg("⚠️ Update channel full, skipping")
						}
					}
				}
			}
		}
	}()

	c.log.Info().Msg("✅ Subscription started (Ctrl+C to stop)")

	// Blocker til context kanselleres
	<-ctx.Done()
	return nil
}
