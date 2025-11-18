package opcua

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua/ua"
)

// ListSymbols returns a static list of known PLC variables for goTØV.
// Beckhoff CX OPC UA servers do not expose these via browse, so we define them manually.
func (c *Client) ListSymbols(ctx context.Context) ([]*ua.NodeID, error) {
	symbols := []string{
		"MAIN.fbUA.bkTemp",
		"MAIN.fbUA.mltTemp",
		"MAIN.fbUA.fermenter1Temp",
		"MAIN.fbUA.fermenter2Temp",
		"MAIN.fbUA.glykolkjolerTemp",
		"MAIN.fbUA.VannInnVentil",
		"MAIN.fbUA.fermenter1Kjoleventil",
		"MAIN.fbUA.fermenter2Kjoleventil",
		"MAIN.fbUA.fermenter1Varmekappe",
		"MAIN.fbUA.fermenter2Varmekappe",
		"MAIN.fbUA.glykolkjolerPumpe",
	}

	var nodes []*ua.NodeID
	displayNames := make(map[string]string)

	for _, s := range symbols {
		id, err := ua.ParseNodeID(fmt.Sprintf("ns=4;s=%s", s))
		if err != nil {
			c.log.Warn().Err(err).Msgf("⚠️ Could not parse node: %s", s)
			continue
		}

		// Try to read DisplayName from UA server
		n := c.conn.Node(id)
		attrs, err := n.Attributes(ctx, ua.AttributeIDDisplayName)
		if err != nil {
			c.log.Warn().Err(err).Msgf("⚠️ Could not read DisplayName for %s", s)
			displayNames[id.String()] = s // fallback
		} else if len(attrs) > 0 {
			val := attrs[0].Value.Value()
			if lt, ok := val.(*ua.LocalizedText); ok {
				displayNames[id.String()] = lt.Text
				c.log.Info().Msgf("🏷  %s → DisplayName = %s", s, lt.Text)
			} else {
				displayNames[id.String()] = s // fallback
				c.log.Debug().Msgf("🟡 %s has non‐localized DisplayName (%T)", s, val)
			}
		} else {
			displayNames[id.String()] = s
			c.log.Debug().Msgf("🔸 %s had no DisplayName entries", s)
		}

		nodes = append(nodes, id)
	}

	// store display name map in client
	c.displayNames = displayNames

	c.log.Info().Msgf("✅ Loaded %d known PLC symbols", len(nodes))
	return nodes, nil
}
