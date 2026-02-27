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

func (e *Engine) run() {
	defer e.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.processAll()
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

	// 2. Check if step duration is finished
	elapsed := time.Since(state.StepStartedAt).Hours()
	if elapsed >= currentStep.DurationHours {
		e.log.Info().
			Int64("id", state.ID).
			Int("from", state.StepIndex).
			Int("to", state.StepIndex+1).
			Msg("⏭️ Advancing to next fermentation step")

		state.StepIndex++
		state.StepStartedAt = time.Now().UTC()

		if state.StepIndex < len(steps) {
			state.TargetTemp = steps[state.StepIndex].Temperature
		} else {
			// Finished all steps
			return e.processOne(state) // Recurse to handle completion
		}

		if err := e.store.UpdateState(*state); err != nil {
			e.log.Error().Err(err).Msg("failed to persist state update")
		}
	}

	// 3. Thermostat Logic
	coolingActive := false
	if e.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tempTag := fmt.Sprintf("ns=4;s=MAIN.fbUA.fermenter%sTemp", state.TankID)
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

		heating := false
		cooling := false
		hysteresis := float32(0.2)
		target := float32(state.TargetTemp)

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
			Msg("🌡️ Thermostat decision")

		e.setTankHardware(state.TankID, cooling, heating)
		coolingActive = cooling
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

	if err := e.client.WriteTag(ctx, valveTag, cooling); err != nil {
		e.log.Error().Err(err).Str("tag", valveTag).Msg("❌ Failed to write cooling valve")
	}
	if err := e.client.WriteTag(ctx, jacketTag, heating); err != nil {
		e.log.Error().Err(err).Str("tag", jacketTag).Msg("❌ Failed to write heating jacket")
	}
}
