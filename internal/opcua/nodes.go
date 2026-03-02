package opcua

import (
	"context"
	"errors"
	"fmt"

	"github.com/gopcua/opcua/ua"
)

// ListSymbols returns a static list of known PLC variables for goTØV.
// Beckhoff CX OPC UA servers do not expose these via browse, so we define them manually.
func (c *Client) ListSymbols(ctx context.Context) ([]*ua.NodeID, error) {
	symbols := []string{
		// Valves
		"MAIN.fbUA.V1_VannInn",
		"MAIN.fbUA.V2_HLT_TilMLT",
		"MAIN.fbUA.V3_MLT_TilBK",
		"MAIN.fbUA.V4_BK_TilHX",
		"MAIN.fbUA.V5_HX_TilDrain",
		"MAIN.fbUA.V6_HX_TilFerment",
		"MAIN.fbUA.V7_SpargeWater",
		"MAIN.fbUA.V8_ChillerIn",
		"MAIN.fbUA.V9_ChillerOut",

		// Tanks & Temps
		"MAIN.fbUA.bkTemp",
		"MAIN.fbUA.mltTemp",
		"MAIN.fbUA.hltResirkTemp",
		"MAIN.fbUA.mltResirkTemp",
		"MAIN.fbUA.heatExchWaterTemp",
		"MAIN.fbUA.heatExchWortTemp",
		"MAIN.fbUA.returnWaterTemp",
		"MAIN.fbUA.supplyWaterTemp",

		// Heaters (Control Outputs)
		"MAIN.fbUA.bkHeaterPower",
		"MAIN.fbUA.hltHeaterPower",

		// Pumps
		"MAIN.fbUA.pumpeHLT",
		"MAIN.fbUA.pumpeWort",

		// Sensors
		"MAIN.fbUA.flowHLT",
		"MAIN.fbUA.flowMLT",
		"MAIN.fbUA.phValue",
		"MAIN.fbUA.spGravSensor",
		"MAIN.fbUA.hxValvePosition",

		// Fermentation & Glycol
		"MAIN.fbUA.fermenter1Temp",
		"MAIN.fbUA.fermenter1Kjoleventil",
		"MAIN.fbUA.fermenter1Varmekappe",
		"MAIN.fbUA.fermenter2Temp",
		"MAIN.fbUA.fermenter2Kjoleventil",
		"MAIN.fbUA.fermenter2Varmekappe",
		"MAIN.fbUA.glykolkjolerTemp",
		"MAIN.fbUA.glykolkjolerPumpe",
	}

	var nodes []*ua.NodeID

	for _, s := range symbols {
		id, err := ua.ParseNodeID(fmt.Sprintf("ns=4;s=%s", s))
		if err != nil {
			c.log.Debug().Err(err).Msgf("⚠️ Could not parse node ID for %s", s)
			continue
		}

		// Try to read DisplayName from UA server to verify existence
		n := c.conn.Node(id)
		attrs, err := n.Attributes(ctx, ua.AttributeIDDisplayName)
		if err != nil {
			if errors.Is(err, ua.StatusBadNodeIDUnknown) {
				continue // Silently skip non-existent nodes to reduce log noise
			}
			c.log.Debug().Err(err).Msgf("⚠️ Could not verify node %s (using fallback)", s)
			c.SetDisplayName(id.String(), s)
		} else if len(attrs) > 0 {
			if dn, ok := attrs[0].Value.Value().(*ua.LocalizedText); ok && dn != nil {
				c.SetDisplayName(id.String(), dn.Text)
			} else {
				c.SetDisplayName(id.String(), s)
			}
		} else {
			c.SetDisplayName(id.String(), s)
		}

		nodes = append(nodes, id)
	}

	c.log.Info().Msgf("✅ Loaded %d known PLC symbols", len(nodes))
	return nodes, nil
}
