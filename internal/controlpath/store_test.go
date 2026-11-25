package controlpath

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func storeAt(t *testing.T, path string) (*sql.DB, *Store) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return db, s
}
func request(rev string) Request {
	return Request{Version: 1, ID: "b037789b-ff3e-48d3-87ea-bbe3c19f8069", Revision: rev, Target: "ads", Confirmed: true}
}
func TestPersistenceReplayAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, s := storeAt(t, path)
	cn := Connections{UA: "connected", ADS: "connected"}
	initial, err := s.Status(ctx, cn)
	if err != nil || initial.Requested != "ua" || initial.Owner != nil || initial.Fresh {
		t.Fatalf("initial %+v %v", initial, err)
	}
	req := request(initial.Revision)
	if err = s.Accept(ctx, req, cn); err != nil {
		t.Fatal(err)
	}
	after, err := s.Status(ctx, cn)
	if err != nil || after.Revision == initial.Revision || after.Requested != "ads" || after.Transition == nil || after.Transition.State != "unknown" || after.Alert == nil || after.Alert.Severity != "high" {
		t.Fatalf("after %+v %v", after, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, s = storeAt(t, path)
	defer db.Close()
	restored, err := s.Status(ctx, Connections{UA: "unknown", ADS: "unknown"})
	if err != nil || restored.Revision != after.Revision || restored.Requested != "ads" || restored.Transition.State != "unknown" || restored.Owner != nil || len(restored.Available) != 0 {
		t.Fatalf("restart %+v %v", restored, err)
	}
	if err = s.Accept(ctx, req, Connections{UA: "unknown", ADS: "unknown"}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	req.Target = "ua"
	if err = s.Accept(ctx, req, cn); !errors.Is(err, ErrConflict) {
		t.Fatalf("idempotency: %v", err)
	}
}
func TestSimultaneousRequestsSerializeAcrossStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, a := storeAt(t, path)
	defer db.Close()
	db2, b := storeAt(t, path)
	defer db2.Close()
	cn := Connections{UA: "connected", ADS: "connected"}
	snap, err := a.Status(ctx, cn)
	if err != nil {
		t.Fatal(err)
	}
	x, y := request(snap.Revision), request(snap.Revision)
	y.ID = "b037789b-ff3e-48d3-87ea-bbe3c19f8070"
	ready := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, v := range []struct {
		s *Store
		r Request
	}{{a, x}, {b, y}} {
		wg.Add(1)
		go func() { defer wg.Done(); <-ready; errs <- v.s.Accept(ctx, v.r, cn) }()
	}
	close(ready)
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrRevision) {
			conflict++
		} else {
			t.Fatalf("unexpected error %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
func TestPendingAtRestartBecomesUnknown(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, s := storeAt(t, path)
	old, err := s.Status(ctx, Connections{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE fermentation_control_path SET requested_path='ads',operation_id='b037789b-ff3e-48d3-87ea-bbe3c19f8069',target_path='ads',transition_state='pending',reason='pending' WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, s = storeAt(t, path)
	defer db.Close()
	v, err := s.Status(ctx, Connections{UA: "unknown", ADS: "unknown"})
	if err != nil || v.Transition == nil || v.Transition.State != "unknown" || v.Alert == nil || v.Revision == old.Revision {
		t.Fatalf("recovered %+v %v", v, err)
	}
}
func TestContractVectorsForIntentOnly(t *testing.T) {
	raw, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Requests []struct {
			Name    string  `json:"name"`
			Request Request `json:"request"`
			Context struct {
				Target string `json:"target_connection"`
			} `json:"context"`
			Status int `json:"http_status"`
		} `json:"requests"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{"accepted_unknown_owner_but_connection_active": true, "disconnected": true, "unknown_connection": true, "handover_not_implemented": true}
	for _, v := range data.Requests {
		if !cases[v.Name] {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			db, s := storeAt(t, filepath.Join(t.TempDir(), "state.db"))
			defer db.Close()
			cn := Connections{UA: "connected", ADS: v.Context.Target}
			snap, e := s.Status(context.Background(), cn)
			if e != nil {
				t.Fatal(e)
			}
			v.Request.Revision = snap.Revision
			e = s.Accept(context.Background(), v.Request, cn)
			if v.Status == 202 && e != nil {
				t.Fatal(e)
			}
			if v.Status == 409 && !errors.Is(e, ErrUnavailable) {
				t.Fatalf("expected unavailable: %v", e)
			}
			after, e := s.Status(context.Background(), cn)
			if e != nil {
				t.Fatal(e)
			}
			if v.Status == 202 && (after.Transition == nil || after.Transition.State != "unknown" || after.Alert == nil || after.Owner != nil) {
				t.Fatalf("bad state %+v", after)
			}
		})
	}
}

func TestFailedPersistenceDoesNotRecordIntentOrReplayKey(t *testing.T) {
	ctx := context.Background()
	db, s := storeAt(t, filepath.Join(t.TempDir(), "state.db"))
	defer db.Close()
	cn := Connections{UA: "connected", ADS: "connected"}
	before, err := s.Status(ctx, cn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER fail_control_update BEFORE UPDATE ON fermentation_control_path BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	req := request(before.Revision)
	if err = s.Accept(ctx, req, cn); err == nil {
		t.Fatal("expected persistence failure")
	}
	after, err := s.Status(ctx, cn)
	if err != nil || after.Revision != before.Revision || after.Requested != before.Requested || after.Transition != nil {
		t.Fatalf("failure changed state %+v %v", after, err)
	}
	if _, err = db.Exec(`DROP TRIGGER fail_control_update`); err != nil {
		t.Fatal(err)
	}
	if err = s.Accept(ctx, req, cn); err != nil {
		t.Fatalf("failed write retained replay key: %v", err)
	}
}

func TestPendingBlocksNewRequestAndReplayStillWins(t *testing.T) {
	ctx := context.Background()
	db, s := storeAt(t, filepath.Join(t.TempDir(), "state.db"))
	defer db.Close()
	cn := Connections{UA: "connected", ADS: "connected"}
	initial, err := s.Status(ctx, cn)
	if err != nil {
		t.Fatal(err)
	}
	req := request(initial.Revision)
	if err = s.Accept(ctx, req, cn); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE fermentation_control_path SET transition_state='pending' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err = s.Accept(ctx, req, Connections{}); err != nil {
		t.Fatalf("replay while pending: %v", err)
	}
	next := req
	next.ID = "b037789b-ff3e-48d3-87ea-bbe3c19f8070"
	if err = s.Accept(ctx, next, cn); !errors.Is(err, ErrInProgress) {
		t.Fatalf("want in-progress error: %v", err)
	}
}
