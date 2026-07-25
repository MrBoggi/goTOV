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

	lastEvaluation time.Time
	lastHistoryLog time.Time
	mu             sync.RWMutex
	state          *BrewhouseState

	pids        map[string]*PIDController
	lastWritten map[string]interface{}
}

func NewEngine(store Store, client *opcua.Client, log zerolog.Logger) *Engine {
	return &Engine{
		store:       store,
		client:      client,
		log:         log,
		stop:        make(chan struct{}),
		pids:        make(map[string]*PIDController),
		lastWritten: make(map[string]interface{}),
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

	// Sanitize state: ensure all expected fields are present (safety against DB schema delta)
	e.sanitizeState(state)

	e.state = state
	e.lastEvaluation = time.Now()
	e.lastHistoryLog = time.Now()

	// Load PID Configs from DB
	if bkPID, err := e.store.GetPIDConfig("BK"); err == nil {
		e.state.BKHeater.PID = *bkPID
		e.state.BKHeater.IsPID = true
	}
	if mltPID, err := e.store.GetPIDConfig("MLT"); err == nil {
		e.state.MLTHeater.PID = *mltPID
		e.state.MLTHeater.IsPID = true
	}

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
		if newState.BKHeater.Mode == ModeManual && e.state.BKHeater.Mode == ModeAuto {
			// If setpoint in newState is 0 (or default), keep the one from Auto
			if newState.BKHeater.Setpoint == 0 {
				newState.BKHeater.Setpoint = e.state.BKHeater.Setpoint
			}
		}
		if newState.MLTHeater.Mode == ModeManual && e.state.MLTHeater.Mode == ModeAuto {
			if newState.MLTHeater.Setpoint == 0 {
				newState.MLTHeater.Setpoint = e.state.MLTHeater.Setpoint
			}
		}
		// Same for proportional valve if applicable
		if newState.ProportionalV.Mode == ModeManual && e.state.ProportionalV.Mode == ModeAuto {
			if newState.ProportionalV.Setpoint == 0 {
				newState.ProportionalV.Setpoint = e.state.ProportionalV.Setpoint
			}
		}
	}

	e.sanitizeState(newState)
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

func (e *Engine) sanitizeState(state *BrewhouseState) {
	if state.Valves == nil {
		state.Valves = make(map[string]*DigitalActuator)
	}
	if state.Pumps == nil {
		state.Pumps = make(map[string]*DigitalActuator)
	}
	if state.BKHeater == nil {
		state.BKHeater = &AnalogActuator{Mode: ModeManual}
	}
	if state.MLTHeater == nil {
		state.MLTHeater = &AnalogActuator{Mode: ModeManual}
	}
	if state.ProportionalV == nil {
		state.ProportionalV = &AnalogActuator{Mode: ModeManual}
	}
	if state.Sensors == nil {
		state.Sensors = make(map[string]float64)
	}
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
	if e.client != nil && !e.client.IsConnected() {
		// Skip evaluation if PLC is offline
		return
	}

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

	// Periodic History Logging (every 60 seconds)
	if now.Sub(e.lastHistoryLog) >= 60*time.Second {
		e.logHistory()
		e.lastHistoryLog = now
	}

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
	if e.state.BKHeater != nil {
		if e.state.BKHeater.Mode == ModeAuto {
			currentTemp := e.state.Sensors["bkTemp"]
			if e.state.BKHeater.IsPID {
				// Use PID
				pid, ok := e.pids["bkHeater"]
				if !ok {
					pid = NewPIDController(e.state.BKHeater.PID.P, e.state.BKHeater.PID.I, e.state.BKHeater.PID.D, 0, 100)
					e.pids["bkHeater"] = pid
				} else {
					// Update coefficients in case they changed
					pid.P = e.state.BKHeater.PID.P
					pid.I = e.state.BKHeater.PID.I
					pid.D = e.state.BKHeater.PID.D
				}
				e.state.BKHeater.Command = pid.Calculate(e.state.BKHeater.Setpoint, currentTemp, deltaSeconds)
			} else {
				// Fallback to Bang-Bang
				if currentTemp < e.state.BKHeater.Setpoint {
					e.state.BKHeater.Command = 100.0
				} else {
					e.state.BKHeater.Command = 0.0
				}
				// Reset PID state if it exists
				if pid, ok := e.pids["bkHeater"]; ok {
					pid.Reset()
				}
			}
		} else {
			// Manual mode, reset PID
			if pid, ok := e.pids["bkHeater"]; ok {
				pid.Reset()
			}
		}

		// SAFETY INTERLOCK: Force 0 if bkLevel < 20L
		bkLevel := e.state.Sensors["bkLevel"]
		if bkLevel < 20.0 {
			if e.state.BKHeater.Command > 0 {
				e.log.Warn().Float64("bkLevel", bkLevel).Msg("⚠️ BK Heater Interlock active: Level below 20L. Forcing heater OFF.")
			}
			e.state.BKHeater.Command = 0.0
		}
	}

	// MLT Heater
	if e.state.MLTHeater != nil {
		if e.state.MLTHeater.Mode == ModeAuto {
			currentTemp := e.state.Sensors["mltTemp"]
			if e.state.MLTHeater.IsPID {
				pid, ok := e.pids["mltHeater"]
				if !ok {
					pid = NewPIDController(e.state.MLTHeater.PID.P, e.state.MLTHeater.PID.I, e.state.MLTHeater.PID.D, 0, 100)
					e.pids["mltHeater"] = pid
				} else {
					pid.P = e.state.MLTHeater.PID.P
					pid.I = e.state.MLTHeater.PID.I
					pid.D = e.state.MLTHeater.PID.D
				}
				e.state.MLTHeater.Command = pid.Calculate(e.state.MLTHeater.Setpoint, currentTemp, deltaSeconds)
			} else {
				if currentTemp < e.state.MLTHeater.Setpoint {
					e.state.MLTHeater.Command = 100.0
				} else {
					e.state.MLTHeater.Command = 0.0
				}
				if pid, ok := e.pids["mltHeater"]; ok {
					pid.Reset()
				}
			}
		} else {
			if pid, ok := e.pids["mltHeater"]; ok {
				pid.Reset()
			}
		}
	}

	// Proportional Valve PID (Optional/Generic)
	if e.state.ProportionalV != nil {
		if e.state.ProportionalV.Mode == ModeAuto && e.state.ProportionalV.IsPID {
			// Note: We need a process value for the proportional valve if it's using PID.
			// For now, let's assume it might be used for something like flow control or pressure.
			// If no PV is defined specifically, we might need to skip or use a default.
			// Since the user didn't specify the PV for the valve, I'll stick to the heater for now
			// or implement a generic placeholder if needed.
			// Given the current sensors, there isn't a clear generic PV for the proportional valve.
		} else if e.state.ProportionalV.Mode == ModeManual {
			if pid, ok := e.pids["proportionalValve"]; ok {
				pid.Reset()
			}
		}
	}
}

func (e *Engine) writeToUA(ctx context.Context) {
	// Digital Valves
	for name, valve := range e.state.Valves {
		tag := fmt.Sprintf("ns=4;s=MAIN.fbUA.%s", name)

		last, ok := e.lastWritten[tag]
		if ok && last == valve.Command {
			continue
		}

		if err := e.client.WriteTag(ctx, tag, valve.Command); err == nil {
			e.lastWritten[tag] = valve.Command
		}
	}

	// Pumps
	for name, pump := range e.state.Pumps {
		tag := fmt.Sprintf("ns=4;s=MAIN.fbUA.%s", name)

		last, ok := e.lastWritten[tag]
		if ok && last == pump.Command {
			continue
		}

		if err := e.client.WriteTag(ctx, tag, pump.Command); err == nil {
			e.lastWritten[tag] = pump.Command
		}
	}

	// BK Heater
	if e.state.BKHeater != nil {
		tag := "ns=4;s=MAIN.fbUA.bkHeaterPower"
		val := float32(e.state.BKHeater.Command)

		last, ok := e.lastWritten[tag]
		if ok && last == val {
			e.log.Debug().Str("tag", tag).Msg("skipping unchanged BK heater command")
		} else if err := e.client.WriteTag(ctx, tag, val); err == nil {
			e.lastWritten[tag] = val
		}
	}

	// MLT Heater (using hltHeaterPower as control output)
	if e.state.MLTHeater != nil {
		tag := "ns=4;s=MAIN.fbUA.hltHeaterPower"
		val := float32(e.state.MLTHeater.Command)

		last, ok := e.lastWritten[tag]
		if ok && last == val {
			e.log.Debug().Str("tag", tag).Msg("skipping unchanged MLT heater command")
		} else if err := e.client.WriteTag(ctx, tag, val); err == nil {
			e.lastWritten[tag] = val
		}
	}
}

func (e *Engine) readSensors(ctx context.Context) {
	// A list of sensors to poll to keep internal state updated
	sensors := map[string]string{
		"bkTemp":            "ns=4;s=MAIN.fbUA.bkTemp",
		"mltTemp":           "ns=4;s=MAIN.fbUA.mltTemp",
		"phValue":           "ns=4;s=MAIN.fbUA.phValue",
		"flowHLT":           "ns=4;s=MAIN.fbUA.flowHLT",
		"flowMLT":           "ns=4;s=MAIN.fbUA.flowMLT",
		"hltResirkTemp":     "ns=4;s=MAIN.fbUA.hltResirkTemp",
		"mltResirkTemp":     "ns=4;s=MAIN.fbUA.mltResirkTemp",
		"spGravSensor":      "ns=4;s=MAIN.fbUA.spGravSensor",
		"hxValvePosition":   "ns=4;s=MAIN.fbUA.hxValvePosition",
		"heatExchWaterTemp": "ns=4;s=MAIN.fbUA.heatExchWaterTemp",
		"heatExchWortTemp":  "ns=4;s=MAIN.fbUA.heatExchWortTemp",
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

func (e *Engine) logHistory() {
	entry := &HistoryEntry{
		Timestamp: time.Now(),
		BKTemp:    e.state.Sensors["bkTemp"],
		MLTTemp:   e.state.Sensors["mltTemp"],
	}

	if e.state.BKHeater != nil {
		entry.BKPadraag = e.state.BKHeater.Command
	}
	if e.state.MLTHeater != nil {
		entry.MLTPadraag = e.state.MLTHeater.Command
	}

	if err := e.store.LogHistory(entry); err != nil {

		e.log.Error().Err(err).Msg("Failed to log brewhouse history")
	}
}
