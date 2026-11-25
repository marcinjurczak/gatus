package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TwiN/gatus/v5/config"
	"github.com/TwiN/gatus/v5/config/endpoint"
	"github.com/TwiN/gatus/v5/config/suite"
	"github.com/TwiN/gatus/v5/storage"
	"github.com/TwiN/gatus/v5/storage/store"
	"github.com/TwiN/gatus/v5/storage/store/common/paging"
)

func TestScheduleDelayedPrune_RemovesLateInsertedOrphansOnSQL(t *testing.T) {
	origInterval := delayedPruneInterval
	delayedPruneInterval = 25 * time.Millisecond
	defer func() {
		cancelDelayedPrune()
		delayedPruneInterval = origInterval
		store.Get().Close()
	}()

	activeEndpoint := &endpoint.Endpoint{Name: "active", Group: "core"}
	orphanedEndpoint := &endpoint.Endpoint{Name: "orphaned", Group: "core"}
	activeSuite := &suite.Suite{Name: "active-suite", Group: "core"}
	orphanedSuite := &suite.Suite{Name: "orphaned-suite", Group: "core"}

	cfg := &config.Config{
		Endpoints: []*endpoint.Endpoint{activeEndpoint},
		Suites:    []*suite.Suite{activeSuite},
		Storage: &storage.Config{
			Type: storage.TypeSQLite,
			Path: filepath.Join(t.TempDir(), "gatus.db"),
		},
	}

	initializeStorage(cfg)
	// Simulate live checks from the new instance and late checks from a terminating old instance.
	_ = store.Get().InsertEndpointResult(activeEndpoint, &endpoint.Result{Success: true, Timestamp: time.Now()})
	_ = store.Get().InsertEndpointResult(orphanedEndpoint, &endpoint.Result{Success: false, Timestamp: time.Now()})
	_ = store.Get().InsertSuiteResult(activeSuite, &suite.Result{Name: activeSuite.Name, Group: activeSuite.Group, Success: true, Timestamp: time.Now()})
	_ = store.Get().InsertSuiteResult(orphanedSuite, &suite.Result{Name: orphanedSuite.Name, Group: orphanedSuite.Group, Success: false, Timestamp: time.Now()})

	scheduleDelayedPrune(cfg)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		epStatuses, err := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		suiteStatuses, err := store.Get().GetAllSuiteStatuses(paging.NewSuiteStatusParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(epStatuses) == 1 && epStatuses[0].Key == activeEndpoint.Key() &&
			len(suiteStatuses) == 1 && suiteStatuses[0].Key == activeSuite.Key() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	epStatuses, _ := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
	suiteStatuses, _ := store.Get().GetAllSuiteStatuses(paging.NewSuiteStatusParams())
	t.Fatalf("expected 1 active endpoint and 1 active suite after delayed prune, got %d endpoints and %d suites", len(epStatuses), len(suiteStatuses))
}

func TestCancelDelayedPrune_StopsBeforeFiring(t *testing.T) {
	origInterval := delayedPruneInterval
	delayedPruneInterval = 5 * time.Second
	defer func() {
		cancelDelayedPrune()
		delayedPruneInterval = origInterval
		store.Get().Close()
	}()

	orphanedEndpoint := &endpoint.Endpoint{Name: "orphaned", Group: "core"}
	cfg := &config.Config{
		Storage: &storage.Config{
			Type: storage.TypeSQLite,
			Path: filepath.Join(t.TempDir(), "gatus.db"),
		},
	}
	initializeStorage(cfg)
	_ = store.Get().InsertEndpointResult(orphanedEndpoint, &endpoint.Result{Success: false, Timestamp: time.Now()})

	scheduleDelayedPrune(cfg)
	startTime := time.Now()
	cancelDelayedPrune()
	if time.Since(startTime) > time.Second {
		t.Fatalf("cancelDelayedPrune took too long: %v", time.Since(startTime))
	}
	statuses, err := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Key != orphanedEndpoint.Key() {
		t.Fatalf("expected orphaned endpoint to remain unpruned after immediate cancel, got %d statuses", len(statuses))
	}
}

func TestScheduleDelayedPrune_SkippedForMemoryStore(t *testing.T) {
	origInterval := delayedPruneInterval
	delayedPruneInterval = 10 * time.Millisecond
	defer func() {
		cancelDelayedPrune()
		delayedPruneInterval = origInterval
		store.Get().Clear()
	}()

	orphanedEndpoint := &endpoint.Endpoint{Name: "orphaned", Group: "core"}
	cfg := &config.Config{
		Storage: &storage.Config{Type: storage.TypeMemory},
	}
	initializeStorage(cfg)
	_ = store.Get().InsertEndpointResult(orphanedEndpoint, &endpoint.Result{Success: false, Timestamp: time.Now()})

	scheduleDelayedPrune(cfg)
	time.Sleep(40 * time.Millisecond)

	statuses, err := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected delayed prune to be skipped for memory store, got %d statuses", len(statuses))
	}
}
