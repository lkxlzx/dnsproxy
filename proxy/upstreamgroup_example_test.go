package proxy_test

import (
	"fmt"
	"log"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

// ExampleUpstreamGroupConfig demonstrates basic usage of upstream groups.
func ExampleUpstreamGroupConfig() {
	// Create a new upstream group configuration
	ugc := proxy.NewUpstreamGroupConfig()

	// Define upstream options
	opts := &upstream.Options{
		Timeout: 5 * time.Second,
	}

	// Create primary group
	primaryUpstreams := []string{"1.1.1.1", "8.8.8.8"}
	var primaryUps []upstream.Upstream
	for _, addr := range primaryUpstreams {
		u, err := upstream.AddressToUpstream(addr, opts)
		if err != nil {
			log.Fatal(err)
		}
		primaryUps = append(primaryUps, u)
	}

	primaryGroup := &proxy.UpstreamGroup{
		Name:      "primary",
		Upstreams: primaryUps,
		Mode:      proxy.UpstreamModeLoadBalance,
		Timeout:   5 * time.Second,
		Enabled:   true,
	}

	// Add the group
	if err := ugc.AddGroup(primaryGroup); err != nil {
		log.Fatal(err)
	}

	// Set as default group
	ugc.DefaultGroup = "primary"

	// Validate configuration
	if err := ugc.Validate(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Upstream group configured successfully")
	// Output: Upstream group configured successfully
}

// ExampleParseUpstreamGroups demonstrates parsing groups from YAML-like structure.
func ExampleParseUpstreamGroups() {
	spec := &proxy.UpstreamGroupsSpec{
		DefaultGroup: "fast",
		Groups: []proxy.UpstreamGroupSpec{
			{
				Name:    "fast",
				Mode:    "load_balance",
				Timeout: "5s",
				Enabled: true,
				Upstreams: []string{
					"1.1.1.1",
					"8.8.8.8",
				},
			},
		},
	}

	opts := &upstream.Options{
		Timeout: 10 * time.Second,
	}

	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatal(err)
	}

	if err := ugc.Validate(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Parsed %d groups\n", len(ugc.Groups))
	// Output: Parsed 1 groups
}

// ExampleUpstreamGroupConfig_GetGroupForDomain demonstrates domain-based group selection.
func ExampleUpstreamGroupConfig_GetGroupForDomain() {
	ugc := proxy.NewUpstreamGroupConfig()

	opts := &upstream.Options{
		Timeout: 5 * time.Second,
	}

	// Create public group
	publicUps := []upstream.Upstream{}
	u1, _ := upstream.AddressToUpstream("1.1.1.1", opts)
	publicUps = append(publicUps, u1)

	publicGroup := &proxy.UpstreamGroup{
		Name:      "public",
		Upstreams: publicUps,
		Mode:      proxy.UpstreamModeLoadBalance,
		Enabled:   true,
	}
	ugc.AddGroup(publicGroup)

	// Create local group
	localUps := []upstream.Upstream{}
	u2, _ := upstream.AddressToUpstream("192.168.1.1", opts)
	localUps = append(localUps, u2)

	localGroup := &proxy.UpstreamGroup{
		Name:      "local",
		Upstreams: localUps,
		Mode:      proxy.UpstreamModeLoadBalance,
		Enabled:   true,
	}
	ugc.AddGroup(localGroup)

	// Set default and domain mappings
	ugc.DefaultGroup = "public"
	ugc.SetDomainGroup("internal.local", "local")

	// Get group for different domains
	group1, _ := ugc.GetGroupForDomain("google.com")
	fmt.Printf("google.com uses: %s\n", group1.Name)

	group2, _ := ugc.GetGroupForDomain("internal.local")
	fmt.Printf("internal.local uses: %s\n", group2.Name)

	// Output:
	// google.com uses: public
	// internal.local uses: local
}

// ExampleParseUpstreamGroupsFromLines demonstrates text-based configuration.
func ExampleParseUpstreamGroupsFromLines() {
	lines := []string{
		"# Primary group",
		"[group:primary:load_balance:5s]",
		"1.1.1.1",
		"8.8.8.8",
		"",
		"# Local group",
		"[group:local:load_balance:3s]",
		"192.168.1.1",
		"",
		"# Domain mappings",
		"[/internal.local/]local",
		"[default]primary",
	}

	opts := &upstream.Options{
		Timeout: 10 * time.Second,
	}

	ugc, err := proxy.ParseUpstreamGroupsFromLines(lines, opts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Configured %d groups\n", len(ugc.Groups))
	fmt.Printf("Default group: %s\n", ugc.DefaultGroup)
	// Output:
	// Configured 2 groups
	// Default group: primary
}

// ExampleUpstreamGroup_modes demonstrates different upstream modes.
func ExampleUpstreamGroup_modes() {
	modes := []proxy.UpstreamMode{
		proxy.UpstreamModeLoadBalance,
		proxy.UpstreamModeParallel,
		proxy.UpstreamModeFastestAddr,
	}

	for _, mode := range modes {
		fmt.Printf("Mode: %s\n", mode)
	}

	// Output:
	// Mode: load_balance
	// Mode: parallel
	// Mode: fastest_addr
}
