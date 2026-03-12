package fermentation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MrBoggi/goTOV/internal/opcua"
	"github.com/rs/zerolog"
)

type Engine struct {
	store  FermentationStore
	client *opcua.Client
	log    zerolog.Logger

	stop chan struct{}
	wg   sync.WaitGroup

	mu     sync.RWMutex
	active map[int64]*FermentationState

	// Glycol load tracking
	glycolMu            sync.Mutex
	pumpLastState       bool
	pumpOnSince         time.Time
	pumpAccumulatedTime time.Duration
	lastGlycolLogTime   time.Time
	currentGlycolLoad   float64
	lastWritten         map[string]interface{}

	// Non-blocking sequences
	seqMu             sync.Mutex
	desiredValves     map[string]bool // tankID -> bool
	actualValves      map[string]bool // tankID -> bool
	actualPump        bool
	pumpTransitioning bool
}

func NewEngine(store FermentationStore, client *opcua.Client, log zerolog.Logger) *Engine {
	return &Engine{
		store:             store,
		client:            client,
		log:               log,
		stop:              make(chan struct{}),
		active:            make(map[int64]*FermentationState),
		lastGlycolLogTime: time.Now(),
		lastWritten:       make(map[string]interface{}),
		desiredValves:     make(map[string]bool),
		actualValves:      make(map[string]bool),
	}
}

func (e *Engine) Start() {
	e.log.Info().Msg("🚀 Starting fermentation engine")
	if err := e.Restore(); err != nil {
		e.log.Error().Err(err).Msg("❌ Failed to restore fermentation states")
	}

	e.wg.Add(1)
	go e.run()
}

func (e *Engine) Stop() {
	e.log.Info().Msg("🛑 Stopping fermentation engine")
	close(e.stop)
	e.wg.Wait()
}

func (e *Engine) Restore() error {
	active, err := e.store.ListActiveFermentations()
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range active {
		// Fetch active events
		events, err := e.store.GetActiveEvents(active[i].ID)
		if err == nil {
			active[i].Events = events
		}

		e.active[active[i].ID] = &active[i]
		e.log.Info().
			Int64("id", active[i].ID).
			Str("tank", active[i].TankID).
			Msg("🔄 Restored active fermentation")
	}

	// Sync with PLC hardware state to avoid startup desync
	if e.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.syncHardwareState(ctx); err != nil {
			e.log.Error().Err(err).Msg("⚠️ Failed to sync hardware state on startup")
		}
	}

	return nil
}

func (e *Engine) syncHardwareState(ctx context.Context) error {
	e.seqMu.Lock()
	defer e.seqMu.Unlock()

	e.log.Info().Msg("🔄 Syncing engine state with PLC hardware...")

	// 1. Sync Pump
	pumpTag := "ns=4;s=MAIN.fbUA.glykolkjolerPumpe"
	val, err := e.client.ReadNodeValue(ctx, pumpTag)
	if err == nil {
		if isOn, ok := val.(bool); ok {
			e.actualPump = isOn
			e.lastWritten[pumpTag] = isOn
			e.log.Debug().Bool("on", isOn).Msg("✅ Synced glycol pump state")
		}
	} else {
		e.log.Warn().Err(err).Str("tag", pumpTag).Msg("failed to sync pump state")
	}

	// 2. Sync Valves & Heaters for all potential tanks
	// We check for tanks mentioned in current active fermentations, but also generic symbols if possible.
	// For simplicity, let's check tanks 1 and 2 as defined in nodes.go
	for _, tankID := range []string{"1", "2"} {
		valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
		jacketTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sVarmekappe", tankID)

		// Sync Valve
		if val, err := e.client.ReadNodeValue(ctx, valveTag); err == nil {
			if isOpen, ok := val.(bool); ok {
				e.actualValves[tankID] = isOpen
				e.lastWritten[valveTag] = isOpen
				e.log.Debug().Str("tank", tankID).Bool("open", isOpen).Msg("✅ Synced cooling valve state")
			}
		}

		// Sync Jacket
		if val, err := e.client.ReadNodeValue(ctx, jacketTag); err == nil {
			if isOn, ok := val.(bool); ok {
				e.lastWritten[jacketTag] = isOn
				e.log.Debug().Str("tank", tankID).Bool("on", isOn).Msg("✅ Synced heating jacket state")
			}
		}
	}

	return nil
}

func (e *Engine) AddFermentation(state *FermentationState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active[state.ID] = state
	e.log.Info().Int64("id", state.ID).Msg("➕ Added fermentation to engine")
}

// RemoveFermentation removes a fermentation from the engine's in-memory active map
// without touching the database. Use this when the DB has already been updated.
func (e *Engine) RemoveFermentation(id int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, id)
	e.log.Info().Int64("id", id).Msg("➖ Removed fermentation from engine map")
}

// UpdateFermentationMode modifies the mode of an active fermentation in memory.
func (e *Engine) UpdateFermentationMode(id int64, mode string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if state, ok := e.active[id]; ok {
		state.Mode = mode
		e.log.Info().Int64("id", id).Str("mode", mode).Msg("🔄 Updated mode in engine memory")
	}
}

// GetActiveStates returns a snapshot of all active fermentation states
// including computed fields like Transitioning.
func (e *Engine) GetActiveStates() []FermentationState {
	e.mu.RLock()
	defer e.mu.RUnlock()

	states := make([]FermentationState, 0, len(e.active))
	for _, s := range e.active {
		states = append(states, *s)
	}
	return states
}

func (e *Engine) StopFermentation(id int64) error {
	e.mu.Lock()
	state, ok := e.active[id]
	if ok {
		delete(e.active, id)
	}
	e.mu.Unlock()

	if !ok {
		// Even if not in active map (e.g. engine restarted and it wasn't running),
		// we should still try to stop it in the store just in case.
		e.log.Warn().Int64("id", id).Msg("⚠️ Fermentation not found in active engine map, stopping in store only")
	}

	if err := e.store.StopFermentation(id); err != nil {
		return err
	}

	if ok {
		// Turn off all hardware for this tank
		e.setTankHardware(state.TankID, false, false)

		// Glycol Pump Interlock: Only run if at least one valve is open
		// A simple check: if no more active fermentations in engine, stop pump
		activeCount := len(e.active)
		e.mu.RUnlock()

		if activeCount == 0 && e.client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := e.client.WriteTag(ctx, "ns=4;s=MAIN.fbUA.glykolkjolerPumpe", false)
			if err != nil {
				e.log.Error().Err(err).Msg("failed to turn off glycol pump after stop")
			}
		}

		e.log.Info().Int64("id", id).Str("tank", state.TankID).Msg("🛑 Stopped fermentation in engine and store (SAFE SHUTDOWN)")
	}
	return nil
}

func (e *Engine) StopByTank(tankID string) error {
	e.mu.Lock()
	var stateID int64
	found := false
	for id, state := range e.active {
		if state.TankID == tankID {
			stateID = id
			found = true
			break
		}
	}
	e.mu.Unlock()

	if !found {
		// Try to find it in the store instead
		state, err := e.store.GetStateByTank(tankID)
		if err != nil {
			return ErrFermentationNotFound
		}
		stateID = state.ID
	}

	return e.StopFermentation(stateID)
}

func (e *Engine) run() {
	defer e.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	glycolTicker := time.NewTicker(1 * time.Minute)
	defer glycolTicker.Stop()

	for {
		select {
		case <-ticker.C:
			e.processAll()
		case <-glycolTicker.C:
			e.logGlycol()
		case <-e.stop:
			return
		}
	}
}

func (e *Engine) processAll() {
	if e.client != nil && !e.client.IsConnected() {
		// Silently skip if PLC is offline to avoid log spam
		return
	}

	e.mu.RLock()
	var toProcess []*FermentationState
	for _, state := range e.active {
		toProcess = append(toProcess, state)
	}
	e.mu.RUnlock()

	anyCooling := false
	for _, state := range toProcess {
		coolingActive, err := e.processOne(state)
		if err != nil {
			e.log.Error().Err(err).Int64("id", state.ID).Msg("❌ Error processing fermentation")
			continue
		}
		if coolingActive {
			anyCooling = true
		}
	}

	// 5. Glycol Pump Interlock: Safe Sequential Coordination
	e.orchestrateGlycol(anyCooling)
}

func (e *Engine) orchestrateGlycol(anyCoolingDesired bool) {
	if e.client == nil {
		return
	}

	e.seqMu.Lock()
	defer e.seqMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Handle Valve Synchronization (Desired -> Actual)
	// We open valves immediately when desired.
	// Closure happens AFTER pump stops in the stop sequence.
	for tankID, desired := range e.desiredValves {
		if desired && !e.actualValves[tankID] {
			valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
			if err := e.client.WriteTag(ctx, valveTag, true); err == nil {
				e.actualValves[tankID] = true
				e.lastWritten[valveTag] = true
			}
		}
	}

	// 2. Handle Pump Sequence
	pumpTag := "ns=4;s=MAIN.fbUA.glykolkjolerPumpe"

	if anyCoolingDesired && !e.actualPump && !e.pumpTransitioning {
		// START SEQUENCE: Valve is already open (step 1), wait 1s then start pump
		e.pumpTransitioning = true
		e.log.Info().Msg("⏳ Cooling requested: Waiting 1s for valves to settle before starting pump")
		time.AfterFunc(1*time.Second, func() {
			e.seqMu.Lock()
			defer e.seqMu.Unlock()
			tCtx, tCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer tCancel()

			if err := e.client.WriteTag(tCtx, pumpTag, true); err == nil {
				e.actualPump = true
				e.lastWritten[pumpTag] = true
			}
			e.pumpTransitioning = false
			e.log.Info().Msg("🚀 Glycol pump STARTED")
		})
	} else if !anyCoolingDesired && e.actualPump && !e.pumpTransitioning {
		// STOP SEQUENCE: Stop pump, wait 1s, then close all valves
		e.pumpTransitioning = true
		e.log.Info().Msg("⏳ Stopping cooling: Stopping pump first")
		if err := e.client.WriteTag(ctx, pumpTag, false); err == nil {
			e.actualPump = false
			e.lastWritten[pumpTag] = false

			e.log.Info().Msg("⏳ Pump stopped, waiting 1s before closing valves")
			time.AfterFunc(1*time.Second, func() {
				e.seqMu.Lock()
				defer e.seqMu.Unlock()
				tCtx, tCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer tCancel()

				for tankID, actual := range e.actualValves {
					if actual && !e.desiredValves[tankID] {
						valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
						if err := e.client.WriteTag(tCtx, valveTag, false); err == nil {
							e.actualValves[tankID] = false
							e.lastWritten[valveTag] = false
						}
					}
				}
				e.pumpTransitioning = false
				e.log.Info().Msg("🔒 All glycol valves CLOSED (safe shutdown)")
			})
		} else {
			e.pumpTransitioning = false // retry next loop if write failed
		}
	} else if !e.pumpTransitioning {
		// FALLBACK RECONCILIATION:
		// 1. Sync any valves that should close while pump is still running
		// (allowed as long as at least one remains open)
		if e.actualPump {
			for tankID, actual := range e.actualValves {
				if actual && !e.desiredValves[tankID] {
					valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
					if err := e.client.WriteTag(ctx, valveTag, false); err == nil {
						e.actualValves[tankID] = false
						e.lastWritten[valveTag] = false
					}
				}
			}
		}

		// 2. CRITICAL FIX: If pump is OFF but valves are OPEN and not desired, close them.
		// This handles cases where pump was turned off externally or sequences were interrupted.
		if !e.actualPump {
			for tankID, actual := range e.actualValves {
				if actual && !e.desiredValves[tankID] {
					valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
					e.log.Warn().Str("tank", tankID).Msg("⚠️ Valve persistence detected while pump is OFF. Closing valve.")
					if err := e.client.WriteTag(ctx, valveTag, false); err == nil {
						e.actualValves[tankID] = false
						e.lastWritten[valveTag] = false
					}
				}
			}
		}
	}

	// 3. Track pump duration for duty-cycle calculation
	e.updatePumpTracking(e.actualPump)
}

func (e *Engine) updatePumpTracking(isCurrentlyOn bool) {
	e.glycolMu.Lock()
	defer e.glycolMu.Unlock()

	now := time.Now()
	if isCurrentlyOn {
		if !e.pumpLastState {
			// Turned ON
			e.pumpOnSince = now
		} else {
			// Stayed ON - accumulate duration since last check
			e.pumpAccumulatedTime += now.Sub(e.pumpOnSince)
			e.pumpOnSince = now
		}
	} else {
		if e.pumpLastState {
			// Turned OFF - record final bit of duration
			e.pumpAccumulatedTime += now.Sub(e.pumpOnSince)
		}
	}
	e.pumpLastState = isCurrentlyOn
}

func (e *Engine) logGlycol() {
	e.mu.RLock()
	activeCount := len(e.active)
	e.mu.RUnlock()

	// Only log if there's an active fermentation
	if activeCount == 0 {
		return
	}

	if e.client == nil || !e.client.IsConnected() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	val, err := e.client.ReadNodeValue(ctx, "ns=4;s=MAIN.fbUA.glykolkjolerTemp")
	if err != nil {
		e.log.Error().Err(err).Msg("failed to read glycol temperature for logging")
		return
	}

	var temp float64
	switch v := val.(type) {
	case float32:
		temp = float64(v)
	case float64:
		temp = v
	default:
		e.log.Warn().Interface("val", val).Msg("unexpected type for glycol temp")
		return
	}

	// Calculate duty cycle since last log
	e.glycolMu.Lock()
	now := time.Now()

	// If pump is currently on, add the time since it started/was last checked
	if e.pumpLastState {
		e.pumpAccumulatedTime += now.Sub(e.pumpOnSince)
		e.pumpOnSince = now
	}

	totalInterval := now.Sub(e.lastGlycolLogTime)
	var load float64
	if totalInterval > 0 {
		load = (float64(e.pumpAccumulatedTime) / float64(totalInterval)) * 100.0
	}
	if load > 100 {
		load = 100
	}

	e.currentGlycolLoad = load
	e.pumpAccumulatedTime = 0
	e.lastGlycolLogTime = now
	e.glycolMu.Unlock()

	pressureValue := 0.0
	pressure := &pressureValue

	if err := e.store.LogGlycolData(temp, pressure, load); err != nil {
		e.log.Error().Err(err).Msg("failed to log glycol history")
	} else {
		e.log.Debug().Float64("temp", temp).Float64("load", load).Msg("📊 Logged glycol history data")
	}
}

// GetGlycolLoad returns the last calculated duty-cycle load percentage
func (e *Engine) GetGlycolLoad() float64 {
	e.glycolMu.Lock()
	defer e.glycolMu.Unlock()
	return e.currentGlycolLoad
}

func (e *Engine) processOne(state *FermentationState) (bool, error) {
	// 1. Get current plan and steps
	steps, err := e.store.GetSteps(state.PlanID)
	if err != nil {
		return false, err
	}

	if state.StepIndex >= len(steps) {
		e.log.Info().Int64("id", state.ID).Msg("✅ Fermentation completed")
		state.Status = StatusCompleted
		_ = e.store.UpdateState(*state)
		e.mu.Lock()
		delete(e.active, state.ID)
		e.mu.Unlock()

		// Ensure we turn off everything for this tank
		e.setTankHardware(state.TankID, false, false)
		return false, nil
	}

	currentStep := steps[state.StepIndex]

	// 2. Thermostat Logic & Transitioning Check
	// We read temperature early to determine if we are "transitioning"
	coolingActive := false
	if e.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tempTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sTemp", state.TankID)
		e.log.Debug().Str("tag", tempTag).Msg("🌡️ Reading current temperature")
		val, err := e.client.ReadNodeValue(ctx, tempTag)
		if err != nil {
			return false, fmt.Errorf("failed to read current temp: %w", err)
		}

		currentTemp, ok := val.(float32)
		if !ok {
			// Try float64
			if v64, ok := val.(float64); ok {
				currentTemp = float32(v64)
			} else {
				return false, fmt.Errorf("current temp is not a float: %T", val)
			}
		}

		target := float32(state.TargetTemp)
		hysteresis := float32(0.2)
		heating := false
		cooling := false

		// Transitioning check: are we within ±0.2°C of setpoint?
		withinRange := currentTemp >= target-hysteresis && currentTemp <= target+hysteresis

		if !state.StepActive {
			if withinRange {
				state.StepActive = true
				state.Transitioning = false
				// Save state since StepActive changed
				if err := e.store.UpdateState(*state); err != nil {
					e.log.Error().Err(err).Msg("failed to update state when step became active")
				}
				e.log.Info().
					Str("tank", state.TankID).
					Float32("current", currentTemp).
					Float32("target", target).
					Msg("🎯 Reached setpoint — step timer has started")
			} else {
				state.Transitioning = true
				// If we are transitioning, we keep resetting StepStartedAt to "now"
				// until we are within range.
				state.StepStartedAt = SQLiteTime{time.Now().UTC()}
				if err := e.store.UpdateState(*state); err != nil {
					e.log.Error().Err(err).Msg("failed to update state during transition")
				}
				e.log.Debug().
					Str("tank", state.TankID).
					Float32("current", currentTemp).
					Float32("target", target).
					Msg("⏳ Transitioning to setpoint — time not counting")
			}
		} else {
			// Even if we fall out of range, transitioning remains false
			state.Transitioning = false
		}

		// 3. Check if step duration is finished (ONLY if StepActive is true)
		if state.StepActive {
			elapsed := time.Since(state.StepStartedAt.Time).Hours()
			if elapsed >= currentStep.DurationHours {
				e.log.Info().
					Int64("id", state.ID).
					Int("from", state.StepIndex).
					Int("to", state.StepIndex+1).
					Msg("⏭️ Advancing to next fermentation step")

				state.StepIndex++
				state.StepStartedAt = SQLiteTime{time.Now().UTC()}
				state.StepActive = false // Reset active flag for the new step

				if state.StepIndex < len(steps) {
					state.TargetTemp = steps[state.StepIndex].Temperature
					// Recurse or just let the next loop handle it?
					// Let's recurse to ensure new target is applied immediately
					return e.processOne(state)
				} else {
					// Finished all steps
					return e.processOne(state) // Recurse to handle completion
				}
			}
		}

		// 4. Update Thermostat States based on currentTemp
		valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", state.TankID)
		jacketTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sVarmekappe", state.TankID)

		if state.Mode == "Manual" {
			// MANUAL MODE: Do not touch hardware, but read state to log and orchestrate glycol pump
			val, err := e.client.ReadNodeValue(ctx, valveTag)
			if err == nil {
				if b, ok := val.(bool); ok {
					coolingActive = b
					// Sync to engine so pump logic knows cooling is demanded
					e.seqMu.Lock()
					e.desiredValves[state.TankID] = b
					e.actualValves[state.TankID] = b
					e.seqMu.Unlock()

					e.mu.Lock()
					e.lastWritten[valveTag] = b
					e.mu.Unlock()
				}
			}

			heatingActive := false
			valH, errH := e.client.ReadNodeValue(ctx, jacketTag)
			if errH == nil {
				if b, ok := valH.(bool); ok {
					heatingActive = b
					e.mu.Lock()
					e.lastWritten[jacketTag] = b
					e.mu.Unlock()
				}
			}

			e.log.Debug().
				Str("tank", state.TankID).
				Float32("current", currentTemp).
				Float32("target", target).
				Bool("cooling", coolingActive).
				Bool("heating", heatingActive).
				Msg("🌡️ Mode is Manual - skipping hardware control")

			// Log to history
			if err := e.store.LogData(state.ID, state.PlanID, state.TankID, state.BatchID, currentTemp, target, coolingActive, heatingActive); err != nil {
				e.log.Error().Err(err).Msg("failed to log fermentation history for manual mode")
			}
		} else {
			// AUTO MODE
			// Get last known states to implement the "stop at target" logic
			e.mu.RLock()
			lastCooling, _ := e.lastWritten[valveTag].(bool)
			lastHeating, _ := e.lastWritten[jacketTag].(bool)
			e.mu.RUnlock()

			if currentTemp > target+hysteresis {
				cooling = true
			} else if currentTemp > target {
				cooling = lastCooling // Stay on if already cooling, but don't start
			}

			if currentTemp < target-hysteresis {
				heating = true
			} else if currentTemp < target {
				heating = lastHeating // Stay on if already heating, but don't start
			}

			e.log.Debug().
				Str("tank", state.TankID).
				Float32("current", currentTemp).
				Float32("target", target).
				Bool("cooling", cooling).
				Bool("heating", heating).
				Bool("lastCooling", lastCooling).
				Bool("lastHeating", lastHeating).
				Bool("transitioning", state.Transitioning).
				Msg("🌡️ Thermostat decision")

			e.setTankHardware(state.TankID, cooling, heating)
			coolingActive = cooling

			// Log to history
			if err := e.store.LogData(state.ID, state.PlanID, state.TankID, state.BatchID, currentTemp, target, cooling, heating); err != nil {
				e.log.Error().Err(err).Msg("failed to log fermentation history for auto mode")
			}
		}
	}

	return coolingActive, nil
}

func (e *Engine) setTankHardware(tankID string, cooling bool, heating bool) {
	if e.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update desired cooling state
	e.seqMu.Lock()
	e.desiredValves[tankID] = cooling
	e.seqMu.Unlock()

	jacketTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sVarmekappe", tankID)

	// Change-only write for Jacket
	lastJacket, okJ := e.lastWritten[jacketTag]
	if !okJ || lastJacket != heating {
		if err := e.client.WriteTag(ctx, jacketTag, heating); err == nil {
			e.lastWritten[jacketTag] = heating
		} else {
			e.log.Error().Err(err).Str("tag", jacketTag).Msg("❌ Failed to write heating jacket")
		}
	}
}
