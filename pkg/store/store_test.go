package store

import (
	"context"
	"testing"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestTargetCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	tgt := &vkg.Target{Pattern: "(?i)test$", Label: "Test", Active: true}
	if err := s.CreateTarget(ctx, tgt); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if tgt.ID == "" {
		t.Fatal("expected ID to be set")
	}

	// List
	targets, err := s.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Pattern != "(?i)test$" {
		t.Errorf("unexpected pattern: %s", targets[0].Pattern)
	}

	// Get
	got, err := s.GetTarget(ctx, tgt.ID)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if got == nil || got.Label != "Test" {
		t.Errorf("unexpected target: %+v", got)
	}

	// Active targets
	activeTargets, err := s.ListActiveTargets(ctx)
	if err != nil {
		t.Fatalf("ListActiveTargets: %v", err)
	}
	if len(activeTargets) != 1 || activeTargets[0].ID != tgt.ID {
		t.Errorf("expected 1 active target %s, got %+v", tgt.ID, activeTargets)
	}

	// Update
	tgt.Label = "Updated"
	if err := s.UpdateTarget(ctx, tgt); err != nil {
		t.Fatalf("UpdateTarget: %v", err)
	}
	got, _ = s.GetTarget(ctx, tgt.ID)
	if got.Label != "Updated" {
		t.Errorf("expected Updated, got %s", got.Label)
	}

	// Delete
	if _, err := s.DeleteTarget(ctx, tgt.ID); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
	got, _ = s.GetTarget(ctx, tgt.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestMatchRecordAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	m := &vkg.Match{
		TargetID:    "t1",
		ClientID:    "c1",
		Hostname:    "host1",
		SeekerID:    1,
		MatchString: "test",
		Key:         vkg.Key{Fingerprint: "fp1", AuthorizedString: "auth1"},
	}
	if err := s.RecordMatch(ctx, m); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected ID to be set")
	}

	matches, err := s.ListMatches(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	got, err := s.GetMatch(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	if got == nil || got.Hostname != "host1" {
		t.Errorf("unexpected match: %+v", got)
	}

	count, err := s.MatchCount(ctx, "")
	if err != nil {
		t.Fatalf("MatchCount: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestClientUpsertAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	c := &vkg.ClientInfo{ID: "c1", Hostname: "host1", Version: "v1", Seekers: 4, Status: "active"}
	if err := s.UpsertClient(ctx, c); err != nil {
		t.Fatalf("UpsertClient: %v", err)
	}

	// Update via upsert
	c.KeyRate = 1234.5
	c.LastSeen = time.Now().UTC()
	if err := s.UpsertClient(ctx, c); err != nil {
		t.Fatalf("UpsertClient update: %v", err)
	}

	clients, err := s.ListClients(ctx)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].KeyRate != 1234.5 {
		t.Errorf("expected key rate 1234.5, got %f", clients[0].KeyRate)
	}
}

func TestMarkOfflineClients(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	old := &vkg.ClientInfo{ID: "old", Hostname: "h", Status: "active", LastSeen: time.Now().UTC().Add(-10 * time.Minute)}
	fresh := &vkg.ClientInfo{ID: "fresh", Hostname: "h", Status: "active", LastSeen: time.Now().UTC()}
	if err := s.UpsertClient(ctx, old); err != nil {
		t.Fatalf("UpsertClient(old): %v", err)
	}
	if err := s.UpsertClient(ctx, fresh); err != nil {
		t.Fatalf("UpsertClient(fresh): %v", err)
	}

	if err := s.MarkOfflineClients(ctx, 5*time.Minute); err != nil {
		t.Fatalf("MarkOfflineClients: %v", err)
	}

	clients, _ := s.ListClients(ctx)
	for _, c := range clients {
		if c.ID == "old" && c.Status != "offline" {
			t.Errorf("expected old client to be offline, got %s", c.Status)
		}
		if c.ID == "fresh" && c.Status != "active" {
			t.Errorf("expected fresh client to be active, got %s", c.Status)
		}
	}
}
