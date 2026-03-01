package brewhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/rs/zerolog"
)

type Engine struct {
	store  Store
	client *opcua.Client
	log    zerolog.Logger

	stop chan struct{}
	wg   sync.WaitGroup

	mu    sync.RWMutex
	state *BrewhouseState

	lastEvaluation time.Time
}

func NewEngine(store Store, client *opcua.Client, log zerolog.Logger) *Engine {
	return &Engine{
		store:  store,
		client: client,
		log:    log,
		stop:   make(chan struct{}),
	}
}

func (e *Engine) Start() {
	e.log.Info().Msg("🚀 Starting brewhouse engine")

	// Load state from DB
	state, err := e.store.GetState()
	if err != nil {
		e.log.Error().Err(err).Msg("❌ Failed to load brewhouse state, using initial state")
		state = InitialState()
	}
	e.state = state
	e.lastEvaluation = time.Now()

	e.wg.Add(1)
	go e.run()
}

func (e *Engine) Stop() {
	e.log.Info().Msg("🛑 Stopping brewhouse engine")
	close(e.stop)
	e.wg.Wait()
}

func (e *Engine) GetStateSnapshot() *BrewhouseState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Deep copy could be better, but for now returning reference is okay if read-only
	// The caller should ideally not mutate this directly.
	return e.state
}

// UpdateState is called by REST API to update actuator modes/commands/setpoints
func (e *Engine) UpdateState(newState *BrewhouseState) error {
	e.mu.Lock()

	// Preserve setpoints when switching Auto -> Manual if they weren't explicitly changed
	// This ensures smooth transitions.
	if e.state != nil {
		if newState.Heater.Mode == ModeManual && e.state.Heater.Mode == ModeAuto {
			// If setpoint in newState is 0 (or default), keep the one from Auto
			if newState.Heater.Setpoint == 0 {
				newState.Heater.Setpoint = e.state.Heater.Setpoint
			}
		}
		// Same for proportional valve if applicable
		if newState.ProportionalV.Mode == ModeManual && e.state.ProportionalV.Mode == ModeAuto {
			if newState.ProportionalV.Setpoint == 0 {
				newState.ProportionalV.Setpoint = e.state.ProportionalV.Setpoint
			}
		}
	}

	e.state = newState
	e.mu.Unlock()

	// Persist changes
	if err := e.store.SaveState(newState); err != nil {
		e.log.Error().Err(err).Msg("Failed to save brewhouse state to DB")
		return err
	}

	// Force an immediate evaluation loop
	e.evaluateAndWrite(context.Background())
	return nil
}

func (e *Engine) run() {
	defer e.wg.Done()

	// Fast loop for brewhouse responsiveness (e.g. 1 second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			e.evaluateAndWrite(ctx)
			cancel()
		case <-e.stop:
			return
		}
	}
}

func (e *Engine) evaluateAndWrite(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Evaluate Logic (updates internal state)
	e.evaluate(ctx)

	// 2. Write to OPC UA if client exists
	if e.client != nil {
		e.writeToUA(ctx)
	}
}

func (e *Engine) evaluate(ctx context.Context) {
	now := time.Now()
	deltaSeconds := now.Sub(e.lastEvaluation).Seconds()
	e.lastEvaluation = now

	// 1. Read Sensors from OPC UA (if exists)
	if e.client != nil {
		e.readSensors(ctx)
	}

	// 2. Calculate Tank Levels (Integration)
	flowHLT := e.state.Sensors["flowHLT"]
	flowMLT := e.state.Sensors["flowMLT"]

	deltaHLT := (flowHLT / 60.0) * deltaSeconds
	deltaMLT := (flowMLT / 60.0) * deltaSeconds

	e.state.Sensors["mltLevel"] += deltaMLT
	e.state.Sensors["bkLevel"] += deltaHLT

	if e.state.Sensors["mltLevel"] < 0 {
		e.state.Sensors["mltLevel"] = 0
	}
	if e.state.Sensors["bkLevel"] < 0 {
		e.state.Sensors["bkLevel"] = 0
	}

	// 3. Evaluate Logic

	// BK Heater with Safety Interlock
	if e.state.Heater != nil {
		if e.state.Heater.Mode == ModeAuto {
			currentTemp := e.state.Sensors["bkTemp"]
			if currentTemp < e.state.Heater.Setpoint {
				e.state.Heater.Command = 100.0
			} else {
				e.state.Heater.Command = 0.0
			}
		}

		// SAFETY INTERLOCK: Force 0 if bkLevel < 20L
		bkLevel := e.state.Sensors["bkLevel"]
		if bkLevel < 20.0 {
			if e.state.Heater.Command > 0 {
				e.log.Warn().Float64("bkLevel", bkLevel).Msg("⚠️ BK Heater Interlock active: Level below 20L. Forcing heater OFF.")
			}
			e.state.Heater.Command = 0.0
		}
	}
}

func (e *Engine) writeToUA(ctx context.Context) {
	// Digital Valves
	for name, valve := range e.state.Valves {
		tag := fmt.Sprintf("ns=4;s=MAIN.fbUA.%s_State", name)
		_ = e.client.WriteTag(ctx, tag, valve.Command)
	}

	// Pumps
	for name, pump := range e.state.Pumps {
		tag := fmt.Sprintf("ns=4;s=MAIN.fbUA.%s_State", name)
		_ = e.client.WriteTag(ctx, tag, pump.Command)
	}

	// Analog Valve PV1
	if e.state.ProportionalV != nil {
		tag := "ns=4;s=MAIN.fbUA.PV1_State"
		_ = e.client.WriteTag(ctx, tag, e.state.ProportionalV.Command)
	}

	// BK Heater
	if e.state.Heater != nil {
		tag := "ns=4;s=MAIN.fbUA.BK_Heater_State"
		_ = e.client.WriteTag(ctx, tag, e.state.Heater.Command)
	}
}

func (e *Engine) readSensors(ctx context.Context) {
	// A list of sensors to poll to keep internal state updated
	sensors := map[string]string{
		"bkTemp":   "ns=4;s=MAIN.fbUA.bkTemp",
		"mltPH":    "ns=4;s=MAIN.fbUA.mltPH",
		"mltLevel": "ns=4;s=MAIN.fbUA.mltLevel",
		"bkLevel":  "ns=4;s=MAIN.fbUA.bkLevel",
		"flowHLT":  "ns=4;s=MAIN.fbUA.flowHLT",
		"flowMLT":  "ns=4;s=MAIN.fbUA.flowMLT",
	}

	for key, tag := range sensors {
		val, err := e.client.ReadNodeValue(ctx, tag)
		if err == nil {
			var floatVal float64
			switch v := val.(type) {
			case float32:
				floatVal = float64(v)
			case float64:
				floatVal = v
			}
			e.state.Sensors[key] = floatVal
		}
	}
}
