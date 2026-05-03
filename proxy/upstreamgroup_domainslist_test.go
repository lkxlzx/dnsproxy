package proxy

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// TestParseDomainLists tests the domains_lists configuration parsing.
func TestParseDomainLists(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	// Create test domain files
	chinaFile := tmpDir + "/china.txt"
	if err := os.WriteFile(chinaFile, []byte("baidu.com\ntaobao.com\n"), 0644); err != nil {
		t.Fatalf("failed to create china file: %v", err)
	}

	overseasFile := tmpDir + "/overseas.txt"
	if err := os.WriteFile(overseasFile, []byte("google.com\nyoutube.com\n"), 0644); err != nil {
		t.Fatalf("failed to create overseas file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create test configuration
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "overseas",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "parallel",
				Timeout:   "10s",
				Enabled:   true,
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:            "china-list",
				Source:          chinaFile,
				Group:           "china",
				File:            tmpDir + "/cache/china.txt",
				AutoUpdate:      true,
				RefreshInterval: "6h",
				Enabled:         true,
			},
			{
				Name:            "overseas-list",
				Source:          overseasFile,
				Group:           "overseas",
				File:            tmpDir + "/cache/overseas.txt",
				AutoUpdate:      true,
				RefreshInterval: "12h",
				Enabled:         true,
			},
		},
		DefaultGroup: "overseas",
		Cache: &CacheConfigSpec{
			Enabled:                true,
			Directory:              tmpDir + "/cache",
			TTL:                    "24h",
			DefaultRefreshInterval: "24h",
			AutoUpdate:             false, // Disable auto-update for test
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Verify groups
	if len(ugc.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(ugc.Groups))
	}

	// Verify domain mappings
	tests := []struct {
		domain        string
		expectedGroup string
	}{
		{"baidu.com", "china"},
		{"taobao.com", "china"},
		{"google.com", "overseas"},
		{"youtube.com", "overseas"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tt.domain)
			if err != nil {
				t.Fatalf("GetGroupForDomain(%q) error: %v", tt.domain, err)
			}

			if group.Name != tt.expectedGroup {
				t.Errorf("GetGroupForDomain(%q) = %q, want %q", tt.domain, group.Name, tt.expectedGroup)
			}
		})
	}
}

// TestParseDomainLists_DisabledList tests that disabled lists are skipped.
func TestParseDomainLists_DisabledList(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := tmpDir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("test.com\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "test",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:    "disabled-list",
				Source:  testFile,
				Group:   "test",
				Enabled: false, // Disabled
			},
		},
		DefaultGroup: "test",
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Verify that no domains were loaded
	if len(ugc.DomainGroups) > 0 {
		t.Errorf("expected no domain mappings, got %d", len(ugc.DomainGroups))
	}
}

// TestParseDomainLists_WithRefreshInterval tests refresh interval parsing.
func TestParseDomainLists_WithRefreshInterval(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := tmpDir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("test.com\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "test",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:            "test-list",
				Source:          testFile,
				Group:           "test",
				File:            tmpDir + "/cache/test.txt",
				AutoUpdate:      true,
				RefreshInterval: "6h",
				Enabled:         true,
			},
		},
		DefaultGroup: "test",
		Cache: &CacheConfigSpec{
			Enabled:   true,
			Directory: tmpDir + "/cache",
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	_, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Success - configuration parsed correctly
}

// TestParseDomainLists_MixedConfiguration tests mixing domain_groups and domains_lists.
func TestParseDomainLists_MixedConfiguration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := tmpDir + "/file-domains.txt"
	if err := os.WriteFile(testFile, []byte("file-domain.com\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "test",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		// Direct domain mappings
		DomainGroups: map[string]interface{}{
			"direct-domain.com": "test",
		},
		// Domain lists
		DomainLists: []DomainListSpec{
			{
				Name:    "file-list",
				Source:  testFile,
				Group:   "test",
				Enabled: true,
			},
		},
		DefaultGroup: "test",
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Verify both direct and file domains are loaded
	tests := []string{"direct-domain.com", "file-domain.com"}
	for _, domain := range tests {
		group, err := ugc.GetGroupForDomain(domain)
		if err != nil {
			t.Fatalf("GetGroupForDomain(%q) error: %v", domain, err)
		}

		if group.Name != "test" {
			t.Errorf("GetGroupForDomain(%q) = %q, want %q", domain, group.Name, "test")
		}
	}
}

// TestDomainListSpec_Validation tests validation of DomainListSpec fields.
func TestDomainListSpec_Validation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name    string
		spec    *UpstreamGroupsSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing source",
			spec: &UpstreamGroupsSpec{
				Groups: []UpstreamGroupSpec{
					{
						Name:      "test",
						Upstreams: []string{"8.8.8.8"},
						Mode:      "load_balance",
						Timeout:   "5s",
						Enabled:   true,
					},
				},
				DomainLists: []DomainListSpec{
					{
						Name:    "test-list",
						Group:   "test",
						Enabled: true,
						// Source is missing
					},
				},
				DefaultGroup: "test",
			},
			wantErr: true,
			errMsg:  "missing source",
		},
		{
			name: "missing group",
			spec: &UpstreamGroupsSpec{
				Groups: []UpstreamGroupSpec{
					{
						Name:      "test",
						Upstreams: []string{"8.8.8.8"},
						Mode:      "load_balance",
						Timeout:   "5s",
						Enabled:   true,
					},
				},
				DomainLists: []DomainListSpec{
					{
						Name:    "test-list",
						Source:  "http://example.com/list.txt",
						Enabled: true,
						// Group is missing
					},
				},
				DefaultGroup: "test",
			},
			wantErr: true,
			errMsg:  "missing group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &upstream.Options{
				Logger:  logger,
				Timeout: 5000,
			}

			_, err := ParseUpstreamGroups(tt.spec, opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUpstreamGroups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseUpstreamGroups() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}
