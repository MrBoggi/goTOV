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
		"MAIN.fbUA.hltTemp",
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
	for _, s := range symbols {
		id, err := ua.ParseNodeID(fmt.Sprintf("ns=4;s=%s", s))
		if err != nil {
			c.log.Warn().Err(err).Msgf("⚠️ Could not parse node: %s", s)
			continue
		}
		nodes = append(nodes, id)
	}

	c.log.Info().Msgf("✅ Loaded %d known PLC symbols", len(nodes))
	return nodes, nil
}
