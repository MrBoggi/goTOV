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

	for _, state := range toProcess {
		if err := e.processOne(state); err != nil {
			e.log.Error().Err(err).Int64("id", state.ID).Msg("❌ Error processing fermentation")
		}
	}
}

func (e *Engine) processOne(state *FermentationState) error {
	// 1. Get current plan and steps
	steps, err := e.store.GetSteps(state.PlanID)
	if err != nil {
		return err
	}

	if state.StepIndex >= len(steps) {
		e.log.Info().Int64("id", state.ID).Msg("✅ Fermentation completed")
		state.Status = StatusCompleted
		_ = e.store.UpdateState(*state)
		e.mu.Lock()
		delete(e.active, state.ID)
		e.mu.Unlock()
		return nil
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

	// 3. Dynamic Tag Mapping
	// Tank 1 -> fermenter1, Tank 2 -> fermenter2
	baseTag := "MAIN.fbUA.fermenter1Temp"
	if state.TankID == "2" {
		baseTag = "MAIN.fbUA.fermenter2Temp"
	}

	// 4. Write to PLC
	if e.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := e.client.WriteTag(ctx, baseTag, state.TargetTemp)
		if err != nil {
			return fmt.Errorf("failed to write target temp to PLC: %w", err)
		}
	}

	return nil
}
