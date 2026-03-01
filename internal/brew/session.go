package brew

import (
	"sync"
	"time"
)

type BrewingSession struct {
	ID            string        `json:"id"`
	StartTime     time.Time     `json:"startTime"`
	Status        string        `json:"status"` // "Manual" or "Brewfather"
	BatchID       string        `json:"batchId,omitempty"`
	RecipeName    string        `json:"recipeName"`
	CurrentStep   string        `json:"currentStep"`
	StepStartTime time.Time     `json:"stepStartTime"`
	StepDuration  time.Duration `json:"stepDuration"`
}

type Engine struct {
	mu      sync.RWMutex
	session *BrewingSession
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) StartSession(session *BrewingSession) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.session = session
}

func (e *Engine) StopSession() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.session = nil
}

func (e *Engine) GetSession() *BrewingSession {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.session
}
