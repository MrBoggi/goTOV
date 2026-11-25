// Package controlpath persists selection intent only. It never accesses a PLC.
package controlpath

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid_request")
var ErrConflict = errors.New("idempotency_conflict")
var ErrRevision = errors.New("revision_conflict")
var ErrUnavailable = errors.New("path_unavailable")
var ErrInProgress = errors.New("transition_in_progress")

type Request struct {
	Version   int    `json:"contract_version"`
	ID        string `json:"request_id"`
	Revision  string `json:"expected_revision"`
	Target    string `json:"target_path"`
	Confirmed bool   `json:"confirmed"`
}
type Transition struct {
	ID     string  `json:"operation_id"`
	Target string  `json:"target_path"`
	State  string  `json:"state"`
	Reason *string `json:"reason"`
}
type Alert struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
}
type Connections struct {
	UA  string `json:"ua"`
	ADS string `json:"ads"`
}
type Snapshot struct {
	Version     int         `json:"contract_version"`
	Revision    string      `json:"revision"`
	Requested   string      `json:"requested_path"`
	Confirmed   *string     `json:"confirmed_path"`
	Owner       *string     `json:"reported_owner"`
	Watchdog    *bool       `json:"watchdog_healthy"`
	ObservedAt  *string     `json:"observed_at"`
	Fresh       bool        `json:"observation_fresh"`
	Connections Connections `json:"connections"`
	Available   []string    `json:"available_paths"`
	Transition  *Transition `json:"transition"`
	Alert       *Alert      `json:"alert"`
}
type Store struct{ db *sql.DB }

func New(ctx context.Context, db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("nil database")
	}
	s := &Store{db: db}
	err := s.transaction(ctx, func(c *sql.Conn) error {
		_, e := c.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS fermentation_control_path (id INTEGER PRIMARY KEY CHECK(id=1), revision TEXT NOT NULL, requested_path TEXT NOT NULL, operation_id TEXT, target_path TEXT, transition_state TEXT, reason TEXT, alert_code TEXT)`)
		if e != nil {
			return e
		}
		_, e = c.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS fermentation_control_requests (request_id TEXT PRIMARY KEY, expected_revision TEXT NOT NULL, target_path TEXT NOT NULL, confirmed INTEGER NOT NULL, contract_version INTEGER NOT NULL)`)
		if e != nil {
			return e
		}
		id, e := uuid.NewRandom()
		if e != nil {
			return e
		}
		_, e = c.ExecContext(ctx, `INSERT OR IGNORE INTO fermentation_control_path(id,revision,requested_path) VALUES(1,?, 'ua')`, id.String())
		if e != nil {
			return e
		}
		id, e = uuid.NewRandom()
		if e != nil {
			return e
		}
		_, e = c.ExecContext(ctx, `UPDATE fermentation_control_path SET revision=?, transition_state='unknown', reason='interrupted_on_restart', alert_code='transition_uncertain' WHERE id=1 AND transition_state='pending'`, id.String())
		return e
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) transaction(ctx context.Context, fn func(*sql.Conn) error) error {
	c, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err = c.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		return err
	}
	if _, err = c.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = c.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if err = fn(c); err != nil {
		return err
	}
	if _, err = c.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}
func normalizeConnection(v string) string {
	if v == "connected" || v == "disconnected" {
		return v
	}
	return "unknown"
}
func (s *Store) Status(ctx context.Context, cn Connections) (Snapshot, error) {
	cn.UA = normalizeConnection(cn.UA)
	cn.ADS = normalizeConnection(cn.ADS)
	var rev, path string
	var id, target, state, reason, alert sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT revision, requested_path,operation_id,target_path,transition_state,reason,alert_code FROM fermentation_control_path WHERE id=1`).Scan(&rev, &path, &id, &target, &state, &reason, &alert)
	if err != nil {
		return Snapshot{}, err
	}
	if rev == "" || (path != "ua" && path != "ads") {
		return Snapshot{}, fmt.Errorf("invalid persisted selection")
	}
	out := Snapshot{Version: 1, Revision: rev, Requested: path, Connections: cn, Available: []string{}}
	if cn.UA == "connected" {
		out.Available = append(out.Available, "ua")
	}
	if cn.ADS == "connected" {
		out.Available = append(out.Available, "ads")
	}
	if state.Valid {
		if !id.Valid || id.String == "" || !target.Valid || (target.String != "ua" && target.String != "ads") || !reason.Valid || reason.String == "" || (state.String != "pending" && state.String != "unknown" && state.String != "failed") || ((state.String == "unknown" || state.String == "failed") && (!alert.Valid || alert.String == "")) {
			return Snapshot{}, fmt.Errorf("invalid operation state")
		}
		r := reason.String
		out.Transition = &Transition{ID: id.String, Target: target.String, State: state.String, Reason: &r}
	}
	if alert.Valid {
		out.Alert = &Alert{Severity: "high", Code: alert.String}
	}
	return out, nil
}
func (s *Store) Accept(ctx context.Context, r Request, cn Connections) error {
	id, err := uuid.Parse(r.ID)
	if err != nil || id.String() != r.ID || r.Version != 1 || r.Revision == "" || (r.Target != "ua" && r.Target != "ads") || !r.Confirmed {
		return ErrInvalid
	}
	return s.transaction(ctx, func(c *sql.Conn) error {
		var prev Request
		err := c.QueryRowContext(ctx, `SELECT request_id,expected_revision,target_path,confirmed,contract_version FROM fermentation_control_requests WHERE request_id=?`, r.ID).Scan(&prev.ID, &prev.Revision, &prev.Target, &prev.Confirmed, &prev.Version)
		if err == nil {
			if prev == r {
				return nil
			}
			return ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var revision string
		var state sql.NullString
		if err = c.QueryRowContext(ctx, `SELECT revision,transition_state FROM fermentation_control_path WHERE id=1`).Scan(&revision, &state); err != nil {
			return err
		}
		if state.Valid && state.String == "pending" {
			return ErrInProgress
		}
		if r.Revision != revision {
			return ErrRevision
		}
		if (r.Target == "ua" && cn.UA != "connected") || (r.Target == "ads" && cn.ADS != "connected") {
			return ErrUnavailable
		}
		// No handover implementation is connected. Persist unknown atomically with
		// intent and replay key: there is no pending side effect to resume.
		_, err = c.ExecContext(ctx, `INSERT INTO fermentation_control_requests(request_id,expected_revision,target_path,confirmed,contract_version) VALUES(?,?,?,?,?)`, r.ID, r.Revision, r.Target, r.Confirmed, r.Version)
		if err != nil {
			return err
		}
		revisionID, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		_, err = c.ExecContext(ctx, `UPDATE fermentation_control_path SET revision=?,requested_path=?,operation_id=?,target_path=?,transition_state='unknown',reason='handover_unavailable',alert_code='transition_uncertain' WHERE id=1`, revisionID.String(), r.Target, r.ID, r.Target)
		return err
	})
}
