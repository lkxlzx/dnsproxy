package proxy

import (
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpstreamGroupConfig(t *testing.T) {
	ugc := NewUpstreamGroupConfig()

	require.NotNil(t, ugc)
	assert.NotNil(t, ugc.Groups)
	assert.NotNil(t, ugc.DomainGroups)
	assert.Empty(t, ugc.Groups)
	assert.Empty(t, ugc.DomainGroups)
}

func TestUpstreamGroupConfig_AddGroup(t *testing.T) {
	// Create a mock upstream for valid test cases
	mockUpstream, err := upstream.AddressToUpstream("1.1.1.1", &upstream.Options{})
	require.NoError(t, err)

	testCases := []struct {
		name        string
		group       *UpstreamGroup
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil_group",
			group:       nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name: "empty_name",
			group: &UpstreamGroup{
				Name:      "",
				Upstreams: []upstream.Upstream{},
			},
			wantErr:     true,
			errContains: "name cannot be empty",
		},
		{
			name: "no_upstreams",
			group: &UpstreamGroup{
				Name:      "test",
				Upstreams: []upstream.Upstream{},
			},
			wantErr:     true,
			errContains: "has no upstreams",
		},
		{
			name: "valid_group",
			group: &UpstreamGroup{
				Name:      "valid",
				Upstreams: []upstream.Upstream{mockUpstream},
				Enabled:   true,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ugc := NewUpstreamGroupConfig()
			err := ugc.AddGroup(tc.group)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpstreamGroupConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func() *UpstreamGroupConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "nil_config",
			setup: func() *UpstreamGroupConfig {
				return nil
			},
			wantErr:     true,
			errContains: "no value",
		},
		{
			name: "no_groups",
			setup: func() *UpstreamGroupConfig {
				return NewUpstreamGroupConfig()
			},
			wantErr:     true,
			errContains: "no upstream groups",
		},
		{
			name: "no_default_group",
			setup: func() *UpstreamGroupConfig {
				ugc := NewUpstreamGroupConfig()
				ugc.Groups["test"] = &UpstreamGroup{
					Name:      "test",
					Upstreams: []upstream.Upstream{},
				}
				return ugc
			},
			wantErr:     true,
			errContains: "default group not specified",
		},
		{
			name: "invalid_mode",
			setup: func() *UpstreamGroupConfig {
				mockUpstream, _ := upstream.AddressToUpstream("1.1.1.1", &upstream.Options{})
				ugc := NewUpstreamGroupConfig()
				ugc.Groups["test"] = &UpstreamGroup{
					Name:      "test",
					Upstreams: []upstream.Upstream{mockUpstream},
					Mode:      "invalid_mode",
				}
				ugc.DefaultGroup = "test"
				return ugc
			},
			wantErr:     true,
			errContains: "invalid mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ugc := tc.setup()
			err := ugc.Validate()

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpstreamGroupConfig_SetDomainGroup(t *testing.T) {
	mockUpstream, err := upstream.AddressToUpstream("1.1.1.1", &upstream.Options{})
	require.NoError(t, err)

	ugc := NewUpstreamGroupConfig()
	ugc.Groups["test"] = &UpstreamGroup{
		Name:      "test",
		Upstreams: []upstream.Upstream{mockUpstream},
	}

	testCases := []struct {
		name        string
		domain      string
		groupName   string
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty_domain",
			domain:      "",
			groupName:   "test",
			wantErr:     true,
			errContains: "domain cannot be empty",
		},
		{
			name:        "empty_group_name",
			domain:      "example.com",
			groupName:   "",
			wantErr:     true,
			errContains: "group name cannot be empty",
		},
		{
			name:        "non_existent_group",
			domain:      "example.com",
			groupName:   "nonexistent",
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:      "valid",
			domain:    "example.com",
			groupName: "test",
			wantErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ugc.SetDomainGroup(tc.domain, tc.groupName)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.groupName, ugc.DomainGroups[tc.domain])
			}
		})
	}
}

func TestUpstreamGroup_Modes(t *testing.T) {
	mockUpstream, err := upstream.AddressToUpstream("1.1.1.1", &upstream.Options{})
	require.NoError(t, err)

	modes := []UpstreamMode{
		UpstreamModeLoadBalance,
		UpstreamModeParallel,
		UpstreamModeFastestAddr,
	}

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			group := &UpstreamGroup{
				Name:      "test",
				Mode:      mode,
				Upstreams: []upstream.Upstream{mockUpstream},
				Timeout:   10 * time.Second,
				Enabled:   true,
			}

			assert.Equal(t, mode, group.Mode)
		})
	}
}
