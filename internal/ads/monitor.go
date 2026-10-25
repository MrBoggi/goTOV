package ads

import (
	"sync"
	"time"
)

// Status implements goTOV-Interfaces/api/ads-status/v1.
type Status struct {
	ContractVersion   int        `json:"contract_version"`
	State             string     `json:"state"`
	Enabled           bool       `json:"enabled"`
	LastValidAt       *time.Time `json:"last_valid_at"`
	AgeMS             *int64     `json:"age_ms"`
	LastSequence      *uint32    `json:"last_sequence"`
	ValidSnapshots    uint64     `json:"valid_snapshots"`
	RejectedSnapshots uint64     `json:"rejected_snapshots"`
	LastRejection     *string    `json:"last_rejection"`
}

type Monitor struct {
	mu              sync.RWMutex
	enabled         bool
	freshness       time.Duration
	lastValid       *time.Time
	lastSequence    *uint32
	valid, rejected uint64
	lastRejection   *string
	connecting      bool
}

func NewMonitor(enabled bool, freshness time.Duration) *Monitor {
	return &Monitor{enabled: enabled, freshness: freshness, connecting: enabled}
}

func (m *Monitor) Accepted(sequence uint32, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastValid = &at
	m.lastSequence = &sequence
	m.valid++
	m.lastRejection = nil
	m.connecting = false
}

func (m *Monitor) Rejected(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rejected++
	m.lastRejection = &code
	m.connecting = false
}

func (m *Monitor) Status(now time.Time) Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := Status{ContractVersion: 1, Enabled: m.enabled, ValidSnapshots: m.valid, RejectedSnapshots: m.rejected, LastValidAt: m.lastValid, LastSequence: m.lastSequence, LastRejection: m.lastRejection}
	if !m.enabled {
		s.State = "disabled"
		s.LastValidAt = nil
		s.LastSequence = nil
		s.LastRejection = nil
		return s
	}
	if m.lastValid == nil {
		if m.connecting {
			s.State = "connecting"
		} else {
			s.State = "degraded"
		}
		return s
	}
	age := now.Sub(*m.lastValid).Milliseconds()
	s.AgeMS = &age
	if age <= m.freshness.Milliseconds() {
		s.State = "healthy"
	} else {
		s.State = "degraded"
	}
	return s
}
