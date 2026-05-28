package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDomainListUpdater(t *testing.T) {
	logger := slog.Default()
	updater := NewDomainListUpdater(logger)
	defer updater.Stop()

	// Create temporary directory for test files
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_domains.txt")

	// Create test HTTP server
	testContent := []byte("example.com\ntest.org\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	// Add source
	source := &DomainListSource{
		FilePath:       testFile,
		URL:            server.URL,
		Format:         "plain",
		UpdateInterval: 0, // No auto-update for test
	}

	err := updater.AddSource("test-group", source)
	if err != nil {
		t.Fatalf("Failed to add source: %v", err)
	}

	// Test manual update
	err = updater.UpdateNow("test-group")
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// Verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != string(testContent) {
		t.Errorf("Content mismatch: expected %s, got %s", testContent, content)
	}

	// Get status
	status := updater.GetStatus()
	if len(status) != 1 {
		t.Errorf("Expected 1 status entry, got %d", len(status))
	}

	groupStatus, ok := status["test-group"]
	if !ok {
		t.Fatal("Status for test-group not found")
	}

	if groupStatus.DomainCount != 2 {
		t.Errorf("Expected 2 domains, got %d", groupStatus.DomainCount)
	}

	if groupStatus.LastChecksum == "" {
		t.Error("Expected non-empty checksum")
	}
}

func TestDomainListUpdaterAutoUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping auto-update test in short mode")
	}

	logger := slog.Default()
	updater := NewDomainListUpdater(logger)
	defer updater.Stop()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "auto_update_domains.txt")

	updateCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("example.com\n"))
	}))
	defer server.Close()

	source := &DomainListSource{
		FilePath:       testFile,
		URL:            server.URL,
		Format:         "plain",
		UpdateInterval: 100 * time.Millisecond, // Fast update for testing
	}

	err := updater.AddSource("auto-test", source)
	if err != nil {
		t.Fatalf("Failed to add source: %v", err)
	}

	// Wait for at least 2 updates
	time.Sleep(250 * time.Millisecond)

	if updateCount < 2 {
		t.Errorf("Expected at least 2 updates, got %d", updateCount)
	}
}

func TestDomainListUpdaterHTTPError(t *testing.T) {
	logger := slog.Default()
	updater := NewDomainListUpdater(logger)
	defer updater.Stop()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "error_test.txt")

	// Server returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	source := &DomainListSource{
		FilePath:       testFile,
		URL:            server.URL,
		Format:         "plain",
		UpdateInterval: 0,
	}

	err := updater.AddSource("error-test", source)
	if err != nil {
		t.Fatalf("Failed to add source: %v", err)
	}

	// Update should fail
	err = updater.UpdateNow("error-test")
	if err == nil {
		t.Error("Expected error for HTTP 500")
	}

	// Check error is recorded
	status := updater.GetStatus()
	groupStatus := status["error-test"]
	if groupStatus.LastError == "" {
		t.Error("Expected error to be recorded")
	}
}

func TestDomainListUpdaterChecksumDetection(t *testing.T) {
	logger := slog.Default()
	updater := NewDomainListUpdater(logger)
	defer updater.Stop()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "checksum_test.txt")

	content := []byte("example.com\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	source := &DomainListSource{
		FilePath:       testFile,
		URL:            server.URL,
		Format:         "plain",
		UpdateInterval: 0,
	}

	err := updater.AddSource("checksum-test", source)
	if err != nil {
		t.Fatalf("Failed to add source: %v", err)
	}

	// First update
	err = updater.UpdateNow("checksum-test")
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	status1 := updater.GetStatus()
	checksum1 := status1["checksum-test"].LastChecksum

	// Second update with same content (should detect no change)
	err = updater.UpdateNow("checksum-test")
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	status2 := updater.GetStatus()
	checksum2 := status2["checksum-test"].LastChecksum

	if checksum1 != checksum2 {
		t.Error("Checksums should be identical for same content")
	}
}
