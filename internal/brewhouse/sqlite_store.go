package brewhouse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Store interface {
	GetState() (*BrewhouseState, error)
	SaveState(state *BrewhouseState) error
	GetBrewingSession() (string, error)
	SaveBrewingSession(data string) error
	LogBrewingSession(data string) error
	Close() error
}

type SQLiteStore struct {
	DB *sqlx.DB
}

var _ Store = (*SQLiteStore)(nil)

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory for sqlite db: %w", err)
		}
	}

	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &SQLiteStore{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS brewhouse_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    state_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS brewing_sessions (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    session_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS brewing_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_json TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
	_, err := s.DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to run schema migration: %w", err)
	}

	// Initialize state if empty
	var count int
	err = s.DB.Get(&count, "SELECT COUNT(*) FROM brewhouse_state WHERE id = 1")
	if err != nil {
		return fmt.Errorf("failed to check state existence: %w", err)
	}

	if count == 0 {
		initial := InitialState()
		data, err := json.Marshal(initial)
		if err != nil {
			return fmt.Errorf("failed to marshal initial state: %w", err)
		}
		_, err = s.DB.Exec("INSERT INTO brewhouse_state (id, state_json) VALUES (1, ?)", string(data))
		if err != nil {
			return fmt.Errorf("failed to insert initial state: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) GetState() (*BrewhouseState, error) {
	var stateJSON string
	err := s.DB.Get(&stateJSON, "SELECT state_json FROM brewhouse_state WHERE id = 1")
	if err != nil {
		return nil, fmt.Errorf("get brewhouse state: %w", err)
	}

	var state BrewhouseState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("unmarshal brewhouse state: %w", err)
	}
	return &state, nil
}

func (s *SQLiteStore) SaveState(state *BrewhouseState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal brewhouse state: %w", err)
	}

	_, err = s.DB.Exec("UPDATE brewhouse_state SET state_json = ? WHERE id = 1", string(data))
	if err != nil {
		return fmt.Errorf("update brewhouse state: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetBrewingSession() (string, error) {
	var sessionJSON string
	err := s.DB.Get(&sessionJSON, "SELECT session_json FROM brewing_sessions WHERE id = 1")
	if err != nil {
		return "", fmt.Errorf("get brewing session: %w", err)
	}
	return sessionJSON, nil
}

func (s *SQLiteStore) SaveBrewingSession(data string) error {
	_, err := s.DB.Exec("INSERT INTO brewing_sessions (id, session_json) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET session_json = excluded.session_json", data)
	if err != nil {
		return fmt.Errorf("save brewing session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LogBrewingSession(data string) error {
	_, err := s.DB.Exec("INSERT INTO brewing_logs (session_json) VALUES (?)", data)
	if err != nil {
		return fmt.Errorf("log brewing session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.DB.Close()
}
