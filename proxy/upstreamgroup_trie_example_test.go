package proxy_test

import (
	"fmt"

	"github.com/AdguardTeam/dnsproxy/proxy"
)

// ExampleDomainTrie_basic demonstrates basic Trie operations
func ExampleDomainTrie_basic() {
	// Create a new domain trie
	trie := proxy.NewDomainTrie()

	// Insert domains
	trie.Insert("example.com", "group1")
	trie.Insert("www.example.com", "group2")
	trie.Insert("api.example.com", "group3")

	// Search for domains
	group, found := trie.Search("www.example.com")
	if found {
		fmt.Printf("Found: %s\n", group)
	}

	if _, found := trie.Search("unknown.com"); !found {
		fmt.Println("Not found: unknown.com")
	}

	// Get size
	fmt.Printf("Total domains: %d\n", trie.Size())

	// Output:
	// Found: group2
	// Not found: unknown.com
	// Total domains: 3
}

// ExampleDomainTrie_wildcards demonstrates wildcard matching
func ExampleDomainTrie_wildcards() {
	trie := proxy.NewDomainTrie()

	// Insert wildcard patterns
	trie.Insert("*.example.com", "wildcard_group")
	trie.Insert("*.cn", "china_group")
	trie.Insert("specific.example.com", "specific_group")

	// Wildcard match
	group, found := trie.Search("www.example.com")
	if found {
		fmt.Printf("www.example.com -> %s\n", group)
	}

	// Exact match overrides wildcard
	group, found = trie.Search("specific.example.com")
	if found {
		fmt.Printf("specific.example.com -> %s\n", group)
	}

	// TLD wildcard
	group, found = trie.Search("baidu.cn")
	if found {
		fmt.Printf("baidu.cn -> %s\n", group)
	}

	// Output:
	// www.example.com -> wildcard_group
	// specific.example.com -> specific_group
	// baidu.cn -> china_group
}

// ExampleDomainTrie_largeScale demonstrates performance with large datasets
func ExampleDomainTrie_largeScale() {
	trie := proxy.NewDomainTrie()

	// Insert 10,000 domains
	for i := 0; i < 10000; i++ {
		domain := fmt.Sprintf("domain%d.example.com", i)
		trie.Insert(domain, "group1")
	}

	// Add wildcard patterns
	trie.Insert("*.cn", "china")
	trie.Insert("*.com", "overseas")

	// Fast lookup (O(m) where m is domain length)
	group, found := trie.Search("domain5000.example.com")
	if found {
		fmt.Printf("Found in 10K domains: %s\n", group)
	}

	// Wildcard lookup
	group, found = trie.Search("baidu.cn")
	if found {
		fmt.Printf("Wildcard match: %s\n", group)
	}

	fmt.Printf("Total domains: %d\n", trie.Size())

	// Output:
	// Found in 10K domains: group1
	// Wildcard match: china
	// Total domains: 10002
}

// ExampleUpstreamGroupConfig_withTrie demonstrates automatic Trie usage
func ExampleUpstreamGroupConfig_withTrie() {
	// Create config (Trie is automatically initialized)
	config := proxy.NewUpstreamGroupConfig()

	// Simulate adding domains (in real usage, domains come from config files)
	// The Trie is automatically updated when domains are added
	config.DomainGroups["baidu.com"] = "china"
	config.DomainGroups["qq.com"] = "china"
	config.DomainGroups["taobao.com"] = "china"
	config.DomainGroups["*.cn"] = "china"

	// Rebuild Trie for fast lookup
	config.RebuildTrie()

	fmt.Printf("Total domains: %d\n", len(config.DomainGroups))
	fmt.Println("Trie automatically optimizes domain matching")

	// Output:
	// Total domains: 4
	// Trie automatically optimizes domain matching
}

// ExampleUpstreamGroupConfig_batchLoad demonstrates batch loading optimization
func ExampleUpstreamGroupConfig_batchLoad() {
	config := proxy.NewUpstreamGroupConfig()

	// Batch load domains (more efficient)
	domains := make(map[string]string)
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("ad%d.com", i)
		domains[domain] = "ads"
	}

	// Add to map directly (skip Trie updates)
	for domain, groupName := range domains {
		config.DomainGroups[domain] = groupName
	}

	// Rebuild Trie once (much faster than 1000 individual inserts)
	config.RebuildTrie()

	fmt.Printf("Total domains: %d\n", len(config.DomainGroups))
	fmt.Println("Batch loading optimized with RebuildTrie()")

	// Output:
	// Total domains: 1000
	// Batch loading optimized with RebuildTrie()
}

// ExampleDomainTrie_delete demonstrates domain deletion
func ExampleDomainTrie_delete() {
	trie := proxy.NewDomainTrie()

	// Insert domains
	trie.Insert("test1.com", "group1")
	trie.Insert("test2.com", "group2")
	trie.Insert("test3.com", "group3")

	fmt.Printf("Before delete: %d domains\n", trie.Size())

	// Delete a domain
	trie.Delete("test2.com")

	fmt.Printf("After delete: %d domains\n", trie.Size())

	// Verify deletion
	if _, found := trie.Search("test2.com"); !found {
		fmt.Println("test2.com deleted successfully")
	}

	// Other domains still exist
	if _, found := trie.Search("test1.com"); found {
		fmt.Println("test1.com still exists")
	}

	// Output:
	// Before delete: 3 domains
	// After delete: 2 domains
	// test2.com deleted successfully
	// test1.com still exists
}

// ExampleDomainTrie_clear demonstrates clearing all domains
func ExampleDomainTrie_clear() {
	trie := proxy.NewDomainTrie()

	// Insert many domains
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		trie.Insert(domain, "group1")
	}

	fmt.Printf("Before clear: %d domains\n", trie.Size())

	// Clear all domains
	trie.Clear()

	fmt.Printf("After clear: %d domains\n", trie.Size())

	// Verify empty
	if _, found := trie.Search("domain0.com"); !found {
		fmt.Println("Trie is empty")
	}

	// Output:
	// Before clear: 100 domains
	// After clear: 0 domains
	// Trie is empty
}
