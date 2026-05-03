package proxy

import (
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// TestValidateGroupName tests group name validation with UTF-8 support.
func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
	}{
		{
			name:      "valid ASCII name",
			groupName: "china",
			wantErr:   false,
		},
		{
			name:      "valid Chinese name",
			groupName: "中国大陆",
			wantErr:   false,
		},
		{
			name:      "valid mixed name",
			groupName: "China中国",
			wantErr:   false,
		},
		{
			name:      "valid with spaces",
			groupName: "中国 大陆",
			wantErr:   false,
		},
		{
			name:      "valid emoji",
			groupName: "🇨🇳中国",
			wantErr:   false,
		},
		{
			name:      "empty name",
			groupName: "",
			wantErr:   true,
		},
		{
			name:      "leading spaces",
			groupName: " china",
			wantErr:   true,
		},
		{
			name:      "trailing spaces",
			groupName: "china ",
			wantErr:   true,
		},
		{
			name:      "control characters",
			groupName: "china\n",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupName(tt.groupName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGroupName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNormalizeGroupName tests group name normalization.
func TestNormalizeGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		want      string
	}{
		{
			name:      "no change needed",
			groupName: "china",
			want:      "china",
		},
		{
			name:      "trim spaces",
			groupName: "  china  ",
			want:      "china",
		},
		{
			name:      "normalize multiple spaces",
			groupName: "中国  大陆",
			want:      "中国 大陆",
		},
		{
			name:      "Chinese characters",
			groupName: "中国大陆",
			want:      "中国大陆",
		},
		{
			name:      "mixed with spaces",
			groupName: "  China  中国  ",
			want:      "China 中国",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeGroupName(tt.groupName)
			if got != tt.want {
				t.Errorf("NormalizeGroupName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUpstreamGroupConfig_ChineseGroupNames tests using Chinese group names.
func TestUpstreamGroupConfig_ChineseGroupNames(t *testing.T) {
	ugc := NewUpstreamGroupConfig()

	// Create a mock upstream for testing
	mockUpstream, err := upstream.AddressToUpstream("8.8.8.8", nil)
	if err != nil {
		t.Fatalf("failed to create mock upstream: %v", err)
	}

	// Create groups with Chinese names
	chinaGroup := &UpstreamGroup{
		Name:      "中国大陆",
		Mode:      UpstreamModeLoadBalance,
		Enabled:   true,
		Upstreams: []upstream.Upstream{mockUpstream},
	}

	overseasGroup := &UpstreamGroup{
		Name:      "海外",
		Mode:      UpstreamModeParallel,
		Enabled:   true,
		Upstreams: []upstream.Upstream{mockUpstream},
	}

	// Add groups
	if err := ugc.AddGroup(chinaGroup); err != nil {
		t.Fatalf("failed to add China group: %v", err)
	}

	if err := ugc.AddGroup(overseasGroup); err != nil {
		t.Fatalf("failed to add overseas group: %v", err)
	}

	// Set default group
	ugc.DefaultGroup = "海外"

	// Set domain mappings with Chinese group names
	testDomains := map[string]string{
		"baidu.com":  "中国大陆",
		"taobao.com": "中国大陆",
		"google.com": "海外",
		"youtube.com": "海外",
	}

	for domain, group := range testDomains {
		if err := ugc.SetDomainGroup(domain, group); err != nil {
			t.Fatalf("failed to set domain %q to group %q: %v", domain, group, err)
		}
	}

	// Validate configuration
	if err := ugc.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Test domain lookups
	tests := []struct {
		domain        string
		expectedGroup string
	}{
		{"baidu.com", "中国大陆"},
		{"taobao.com", "中国大陆"},
		{"google.com", "海外"},
		{"youtube.com", "海外"},
		{"unknown.com", "海外"}, // Should use default group
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

// TestUpstreamGroupConfig_MixedLanguageNames tests mixed language group names.
func TestUpstreamGroupConfig_MixedLanguageNames(t *testing.T) {
	ugc := NewUpstreamGroupConfig()

	// Create a mock upstream
	mockUpstream, err := upstream.AddressToUpstream("8.8.8.8", nil)
	if err != nil {
		t.Fatalf("failed to create mock upstream: %v", err)
	}

	// Create groups with mixed language names
	groups := []*UpstreamGroup{
		{
			Name:      "China中国",
			Mode:      UpstreamModeLoadBalance,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
		{
			Name:      "Overseas海外",
			Mode:      UpstreamModeParallel,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
		{
			Name:      "AdBlock广告拦截",
			Mode:      UpstreamModeLoadBalance,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
	}

	for _, group := range groups {
		if err := ugc.AddGroup(group); err != nil {
			t.Fatalf("failed to add group %q: %v", group.Name, err)
		}
	}

	ugc.DefaultGroup = "Overseas海外"

	// Validate
	if err := ugc.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Verify groups exist
	for _, group := range groups {
		if _, exists := ugc.Groups[group.Name]; !exists {
			t.Errorf("group %q not found", group.Name)
		}
	}
}

// TestUpstreamGroupConfig_EmojiInGroupNames tests emoji in group names.
func TestUpstreamGroupConfig_EmojiInGroupNames(t *testing.T) {
	ugc := NewUpstreamGroupConfig()

	// Create a mock upstream
	mockUpstream, err := upstream.AddressToUpstream("8.8.8.8", nil)
	if err != nil {
		t.Fatalf("failed to create mock upstream: %v", err)
	}

	// Create groups with emoji
	groups := []*UpstreamGroup{
		{
			Name:      "🇨🇳中国",
			Mode:      UpstreamModeLoadBalance,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
		{
			Name:      "🌍海外",
			Mode:      UpstreamModeParallel,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
		{
			Name:      "🛡️安全",
			Mode:      UpstreamModeLoadBalance,
			Enabled:   true,
			Upstreams: []upstream.Upstream{mockUpstream},
		},
	}

	for _, group := range groups {
		if err := ugc.AddGroup(group); err != nil {
			t.Fatalf("failed to add group %q: %v", group.Name, err)
		}
	}

	ugc.DefaultGroup = "🌍海外"

	// Validate
	if err := ugc.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Test domain mapping
	if err := ugc.SetDomainGroup("baidu.com", "🇨🇳中国"); err != nil {
		t.Fatalf("failed to set domain: %v", err)
	}

	group, err := ugc.GetGroupForDomain("baidu.com")
	if err != nil {
		t.Fatalf("GetGroupForDomain error: %v", err)
	}

	if group.Name != "🇨🇳中国" {
		t.Errorf("GetGroupForDomain() = %q, want %q", group.Name, "🇨🇳中国")
	}
}

// BenchmarkGetGroupForDomain_ChineseNames benchmarks domain lookup with Chinese names.
func BenchmarkGetGroupForDomain_ChineseNames(b *testing.B) {
	ugc := NewUpstreamGroupConfig()

	// Setup
	chinaGroup := &UpstreamGroup{
		Name:      "中国大陆",
		Mode:      UpstreamModeLoadBalance,
		Enabled:   true,
		Upstreams: []upstream.Upstream{},
	}

	overseasGroup := &UpstreamGroup{
		Name:      "海外",
		Mode:      UpstreamModeParallel,
		Enabled:   true,
		Upstreams: []upstream.Upstream{},
	}

	ugc.AddGroup(chinaGroup)
	ugc.AddGroup(overseasGroup)
	ugc.DefaultGroup = "海外"

	ugc.SetDomainGroup("baidu.com", "中国大陆")
	ugc.SetDomainGroup("*.baidu.com", "中国大陆")
	ugc.SetDomainGroup("google.com", "海外")
	ugc.SetDomainGroup("*.google.com", "海外")

	ugc.RebuildTrie()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = ugc.GetGroupForDomain("www.baidu.com")
	}
}
