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
}

func NewEngine(store FermentationStore, client *opcua.Client, log zerolog.Logger) *Engine {
	return &Engine{
		store:  store,
		client: client,
		log:    log,
		stop:   make(chan struct{}),
		active: make(map[int64]*FermentationState),
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
		e.active[active[i].ID] = &active[i]
		e.log.Info().
			Int64("id", active[i].ID).
			Str("tank", active[i].TankID).
			Msg("🔄 Restored active fermentation")
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

	// 5. Glycol Pump Interlock: Only run if at least one valve is open
	if e.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := e.client.WriteTag(ctx, "ns=4;s=MAIN.fbUA.glykolkjolerPumpe", anyCooling)
		if err != nil {
			e.log.Error().Err(err).Msg("failed to sync glycol pump")
		}
	}
}

func (e *Engine) logGlycol() {
	e.mu.RLock()
	activeCount := len(e.active)
	e.mu.RUnlock()

	// Only log if there's an active fermentation
	if activeCount == 0 {
		return
	}

	if e.client == nil {
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

	// For now, load is 0 and pressure is 0 as per user instructions
	load := 0.0
	pressureValue := 0.0
	pressure := &pressureValue

	if err := e.store.LogGlycolData(temp, pressure, load); err != nil {
		e.log.Error().Err(err).Msg("failed to log glycol history")
	} else {
		e.log.Debug().Float64("temp", temp).Msg("📊 Logged glycol history data")
	}
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
		if currentTemp > target+hysteresis {
			cooling = true
		} else if currentTemp < target-hysteresis {
			heating = true
		}

		e.log.Debug().
			Str("tank", state.TankID).
			Float32("current", currentTemp).
			Float32("target", target).
			Bool("cooling", cooling).
			Bool("heating", heating).
			Bool("transitioning", state.Transitioning).
			Msg("🌡️ Thermostat decision")

		e.setTankHardware(state.TankID, cooling, heating)
		coolingActive = cooling

		// Log to history
		if err := e.store.LogData(state.PlanID, state.TankID, state.BatchID, currentTemp, target, cooling, heating); err != nil {
			e.log.Error().Err(err).Msg("failed to log fermentation history")
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

	valveTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sKjoleventil", tankID)
	jacketTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sVarmekappe", tankID)

	// Diagnostic: read current type before writing
	if val, err := e.client.ReadNodeValue(ctx, jacketTag); err == nil {
		e.log.Debug().Str("tag", jacketTag).Interface("value", val).Msgf("🔎 Tag type diagnostic: %T", val)
	}

	if err := e.client.WriteTag(ctx, valveTag, cooling); err != nil {
		e.log.Error().Err(err).Str("tag", valveTag).Msg("❌ Failed to write cooling valve")
	}
	if err := e.client.WriteTag(ctx, jacketTag, heating); err != nil {
		e.log.Error().Err(err).Str("tag", jacketTag).Msg("❌ Failed to write heating jacket")
	}
}
