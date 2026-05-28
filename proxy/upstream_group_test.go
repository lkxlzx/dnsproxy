package proxy

import (
	"testing"
)

func TestUpstreamGroupManager(t *testing.T) {
	mgr := NewUpstreamGroupManager()

	// Test adding groups with fallback
	group1 := &UpstreamGroup{
		ID:                "china-dns",
		Upstreams:         []string{"223.5.5.5", "119.29.29.29"},
		FallbackUpstreams: []string{"114.114.114.114"},
		Description:       "China DNS servers with fallback",
	}

	err := mgr.AddGroup(group1)
	if err != nil {
		t.Fatalf("Failed to add group: %v", err)
	}

	// Test retrieving group
	retrieved, ok := mgr.GetGroup("china-dns")
	if !ok {
		t.Fatal("Group not found")
	}

	if retrieved.ID != "china-dns" {
		t.Errorf("Expected ID 'china-dns', got '%s'", retrieved.ID)
	}

	if len(retrieved.Upstreams) != 2 {
		t.Errorf("Expected 2 upstreams, got %d", len(retrieved.Upstreams))
	}

	if len(retrieved.FallbackUpstreams) != 1 {
		t.Errorf("Expected 1 fallback upstream, got %d", len(retrieved.FallbackUpstreams))
	}

	// Test getting primary upstreams
	upstreams, err := mgr.GetUpstreams("china-dns")
	if err != nil {
		t.Fatalf("Failed to get upstreams: %v", err)
	}

	if len(upstreams) != 2 {
		t.Errorf("Expected 2 primary upstreams, got %d", len(upstreams))
	}

	// Test getting fallback upstreams
	fallbacks, err := mgr.GetFallbackUpstreams("china-dns")
	if err != nil {
		t.Fatalf("Failed to get fallback upstreams: %v", err)
	}

	if len(fallbacks) != 1 {
		t.Errorf("Expected 1 fallback upstream, got %d", len(fallbacks))
	}

	// Test getting all upstreams
	primary, fallback, err := mgr.GetAllUpstreams("china-dns")
	if err != nil {
		t.Fatalf("Failed to get all upstreams: %v", err)
	}

	if len(primary) != 2 {
		t.Errorf("Expected 2 primary upstreams, got %d", len(primary))
	}

	if len(fallback) != 1 {
		t.Errorf("Expected 1 fallback upstream, got %d", len(fallback))
	}

	// Test listing groups
	ids := mgr.ListGroups()
	if len(ids) != 1 {
		t.Errorf("Expected 1 group, got %d", len(ids))
	}

	// Test resolving upstreams with group ID
	resolvedPrimary, resolvedFallback, err := mgr.ResolveUpstreams("china-dns", nil)
	if err != nil {
		t.Fatalf("Failed to resolve upstreams: %v", err)
	}

	if len(resolvedPrimary) != 2 {
		t.Errorf("Expected 2 resolved primary upstreams, got %d", len(resolvedPrimary))
	}

	if len(resolvedFallback) != 1 {
		t.Errorf("Expected 1 resolved fallback upstream, got %d", len(resolvedFallback))
	}

	// Test resolving upstreams with direct addresses (no fallback)
	direct := []string{"8.8.8.8", "1.1.1.1"}
	resolvedPrimary, resolvedFallback, err = mgr.ResolveUpstreams("", direct)
	if err != nil {
		t.Fatalf("Failed to resolve direct upstreams: %v", err)
	}

	if len(resolvedPrimary) != 2 {
		t.Errorf("Expected 2 resolved primary upstreams, got %d", len(resolvedPrimary))
	}

	if resolvedFallback != nil {
		t.Errorf("Expected no fallback for direct upstreams, got %d", len(resolvedFallback))
	}

	// Test removing group
	removed := mgr.RemoveGroup("china-dns")
	if !removed {
		t.Error("Failed to remove group")
	}

	_, ok = mgr.GetGroup("china-dns")
	if ok {
		t.Error("Group should have been removed")
	}
}

func TestUpstreamGroupValidation(t *testing.T) {
	mgr := NewUpstreamGroupManager()

	// Test empty ID
	err := mgr.AddGroup(&UpstreamGroup{
		ID:        "",
		Upstreams: []string{"8.8.8.8"},
	})
	if err == nil {
		t.Error("Expected error for empty ID")
	}

	// Test empty upstreams
	err = mgr.AddGroup(&UpstreamGroup{
		ID:        "test",
		Upstreams: []string{},
	})
	if err == nil {
		t.Error("Expected error for empty upstreams")
	}

	// Test non-existent group
	_, err = mgr.GetUpstreams("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent group")
	}
}
