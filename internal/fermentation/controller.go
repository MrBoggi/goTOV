package fermentation

import (
	"context"
	"time"

	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/rs/zerolog"
)

type Controller struct {
	Log     zerolog.Logger
	UA      *opcua.Client
	Store   *SQLiteStore
	State   EngineState
	Plan    FermentationState
	Config  EngineConfig
	Ticker  *time.Ticker
	Running bool
}

func NewController(
	log zerolog.Logger,
	ua *opcua.Client,
	store *SQLiteStore,
	state FermentationState,
	cfg EngineConfig,
) *Controller {
	return &Controller{
		Log:    log,
		UA:     ua,
		Store:  store,
		State:  NewEngineState(),
		Plan:   state,
		Config: cfg,
	}
}

func (c *Controller) Start() {
	if c.Running {
		return
	}
	c.Running = true

	c.Ticker = time.NewTicker(1 * time.Second)

	go func() {
		for now := range c.Ticker.C {
			c.tick(now)
		}
	}()
}

func (c *Controller) Stop() {
	if !c.Running {
		return
	}
	c.Running = false
	c.Ticker.Stop()
}

// Main tick:
// 1) Read PLC temperature inputs
// 2) Fetch target temperature via fermentation plan (SQLite)
// 3) Run process engine
// 4) Write outputs to PLC
func (c *Controller) tick(now time.Time) {
	ctx := context.Background()

	// 1) Read from PLC
	t1Temp := c.readFloat(ctx, "MAIN.fbUA.fermenter1Temp")
	t2Temp := c.readFloat(ctx, "MAIN.fbUA.fermenter2Temp")

	// 2) Determine active step & target temperature
	targetTemp, err := c.Store.GetTargetTemperature(c.Plan, now)
	if err != nil {
		c.Log.Error().Err(err).Msg("failed to determine active fermentation step")
		return
	}

	// Inputs
	t1In := TankInput{Active: true, Temp: t1Temp, TargetTemp: targetTemp}
	t2In := TankInput{Active: true, Temp: t2Temp, TargetTemp: targetTemp}

	// 3) Run engine
	c.State = c.State.Tick(c.Config, now, t1In, t2In)

	// 4) Write to PLC
	c.writeBool(ctx, "MAIN.fbUA.fermenter1Kjoleventil", c.State.Outputs.Tanks[Tank1].Cooling)
	c.writeBool(ctx, "MAIN.fbUA.fermenter1Varmekappe", c.State.Outputs.Tanks[Tank1].Heating)
	c.writeBool(ctx, "MAIN.fbUA.fermenter2Kjoleventil", c.State.Outputs.Tanks[Tank2].Cooling)
	c.writeBool(ctx, "MAIN.fbUA.fermenter2Varmekappe", c.State.Outputs.Tanks[Tank2].Heating)

	// Shared glycol pump
	c.writeBool(ctx, "MAIN.fbUA.glykolkjolerPumpe", c.State.Outputs.Pump)
}

func (c *Controller) readFloat(ctx context.Context, node string) float64 {
	val, err := c.UA.ReadNodeValue(ctx, node)
	if err != nil {
		c.Log.Error().Err(err).Msgf("UA read failed (%s)", node)
		return 0
	}

	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	}

	c.Log.Warn().Msgf("unexpected OPC UA type for %s", node)
	return 0
}

func (c *Controller) writeBool(ctx context.Context, node string, val bool) {
	if err := c.UA.WriteNodeValue(ctx, node, val); err != nil {
		c.Log.Error().Err(err).Msgf("UA write failed (%s)", node)
	}
}
