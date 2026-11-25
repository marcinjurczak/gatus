package main

import (
	"testing"
	"time"

	"github.com/TwiN/gatus/v5/config"
	"github.com/TwiN/gatus/v5/config/endpoint"
	"github.com/TwiN/gatus/v5/storage"
	"github.com/TwiN/gatus/v5/storage/store"
	"github.com/TwiN/gatus/v5/storage/store/common/paging"
)

func TestScheduleDelayedPrune_RemovesLateInsertedOrphanedEndpoints(t *testing.T) {
	origInterval := delayedPruneInterval
	delayedPruneInterval = 25 * time.Millisecond
	defer func() {
		cancelDelayedPrune()
		delayedPruneInterval = origInterval
		store.Get().Clear()
	}()

	activeEndpoint := &endpoint.Endpoint{Name: "active", Group: "core"}
	orphanedEndpoint := &endpoint.Endpoint{Name: "orphaned", Group: "core"}
	cfg := &config.Config{
		Endpoints: []*endpoint.Endpoint{activeEndpoint},
		Storage:   &storage.Config{Type: storage.TypeMemory},
	}

	initializeStorage(cfg)
	// Simulate a live check from the new pod and a late check from a terminating old pod.
	_ = store.Get().InsertEndpointResult(activeEndpoint, &endpoint.Result{Success: true, Timestamp: time.Now()})
	_ = store.Get().InsertEndpointResult(orphanedEndpoint, &endpoint.Result{Success: false, Timestamp: time.Now()})

	scheduleDelayedPrune(cfg)
	// Wait for the delayed prune to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statuses, err := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) == 1 && statuses[0].Key == activeEndpoint.Key() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses, _ := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams())
	t.Fatalf("expected only active endpoint to remain after delayed prune, got %d statuses", len(statuses))
}

func TestCancelDelayedPrune_StopsBeforeFiring(t *testing.T) {
	origInterval := delayedPruneInterval
	delayedPruneInterval = 5 * time.Second
	defer func() {
		cancelDelayedPrune()
		delayedPruneInterval = origInterval
		store.Get().Clear()
	}()

	cfg := &config.Config{
		Storage: &storage.Config{Type: storage.TypeMemory},
	}
	initializeStorage(cfg)
	scheduleDelayedPrune(cfg)
	// Cancelling immediately should return promptly without waiting for delayedPruneInterval.
	startTime := time.Now()
	cancelDelayedPrune()
	if time.Since(startTime) > time.Second {
		t.Fatalf("cancelDelayedPrune took too long: %v", time.Since(startTime))
	}
}
