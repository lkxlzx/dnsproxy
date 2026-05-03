package proxy

import (
	"fmt"
	"testing"
)

func TestRadixTree_BasicOperations(t *testing.T) {
	rt := NewRadixTree()

	t.Run("Insert and Search Exact", func(t *testing.T) {
		rt.Insert("example.com", "group1")
		rt.Insert("test.com", "group2")

		if group, found := rt.Search("example.com"); !found || group != "group1" {
			t.Errorf("Search(example.com) = %q, %v; want group1, true", group, found)
		}

		if group, found := rt.Search("test.com"); !found || group != "group2" {
			t.Errorf("Search(test.com) = %q, %v; want group2, true", group, found)
		}

		if _, found := rt.Search("unknown.com"); found {
			t.Error("Search(unknown.com) should not find anything")
		}
	})

	t.Run("Insert and Search Wildcard", func(t *testing.T) {
		rt.Clear()
		rt.Insert("*.example.com", "wildcard_group")

		if group, found := rt.Search("www.example.com"); !found || group != "wildcard_group" {
			t.Errorf("Search(www.example.com) = %q, %v; want wildcard_group, true", group, found)
		}

		if group, found := rt.Search("api.example.com"); !found || group != "wildcard_group" {
			t.Errorf("Search(api.example.com) = %q, %v; want wildcard_group, true", group, found)
		}
	})

	t.Run("Exact Match Priority", func(t *testing.T) {
		rt.Clear()
		rt.Insert("*.example.com", "wildcard_group")
		rt.Insert("www.example.com", "exact_group")

		// Exact match should take priority
		if group, found := rt.Search("www.example.com"); !found || group != "exact_group" {
			t.Errorf("Search(www.example.com) = %q, %v; want exact_group, true", group, found)
		}

		// Other subdomains should match wildcard
		if group, found := rt.Search("api.example.com"); !found || group != "wildcard_group" {
			t.Errorf("Search(api.example.com) = %q, %v; want wildcard_group, true", group, found)
		}
	})
}

func TestRadixTree_WildcardMatching(t *testing.T) {
	rt := NewRadixTree()

	rt.Insert("*.example.com", "example_wildcard")
	rt.Insert("*.api.example.com", "api_wildcard")
	rt.Insert("*.cn", "cn_wildcard")
	rt.Insert("specific.example.com", "specific")

	tests := []struct {
		domain   string
		expected string
		found    bool
	}{
		{"www.example.com", "example_wildcard", true},
		{"test.example.com", "example_wildcard", true},
		{"v1.api.example.com", "api_wildcard", true},
		{"v2.api.example.com", "api_wildcard", true},
		{"specific.example.com", "specific", true},
		{"baidu.cn", "cn_wildcard", true},
		{"qq.cn", "cn_wildcard", true},
		{"unknown.org", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			group, found := rt.Search(tt.domain)
			if found != tt.found {
				t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
			}
			if group != tt.expected {
				t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
			}
		})
	}
}

func TestRadixTree_Delete(t *testing.T) {
	rt := NewRadixTree()

	rt.Insert("example.com", "group1")
	rt.Insert("*.example.com", "group2")

	// Delete exact match
	rt.Delete("example.com")
	if _, found := rt.Search("example.com"); found {
		t.Error("example.com should be deleted")
	}

	// Wildcard should still work
	if group, found := rt.Search("www.example.com"); !found || group != "group2" {
		t.Errorf("www.example.com should still match wildcard")
	}

	// Delete wildcard
	rt.Delete("*.example.com")
	if _, found := rt.Search("www.example.com"); found {
		t.Error("www.example.com should not match after wildcard deletion")
	}
}

func TestRadixTree_Size(t *testing.T) {
	rt := NewRadixTree()

	if size := rt.Size(); size != 0 {
		t.Errorf("Initial size = %d, want 0", size)
	}

	rt.Insert("example.com", "group1")
	if size := rt.Size(); size != 1 {
		t.Errorf("Size after 1 insert = %d, want 1", size)
	}

	rt.Insert("*.example.com", "group2")
	if size := rt.Size(); size != 2 {
		t.Errorf("Size after 2 inserts = %d, want 2", size)
	}

	rt.Delete("example.com")
	if size := rt.Size(); size != 1 {
		t.Errorf("Size after 1 delete = %d, want 1", size)
	}
}

func TestRadixTree_Stats(t *testing.T) {
	rt := NewRadixTree()

	rt.Insert("example.com", "group1")
	rt.Insert("test.com", "group2")
	rt.Insert("*.example.com", "group3")
	rt.Insert("*.cn", "group4")

	stats := rt.Stats()

	if stats.ExactMatches != 2 {
		t.Errorf("ExactMatches = %d, want 2", stats.ExactMatches)
	}

	if stats.WildcardPatterns != 2 {
		t.Errorf("WildcardPatterns = %d, want 2", stats.WildcardPatterns)
	}

	if stats.TotalDomains != 4 {
		t.Errorf("TotalDomains = %d, want 4", stats.TotalDomains)
	}
}

func TestRadixTree_ConcurrentAccess(t *testing.T) {
	rt := NewRadixTree()

	// Insert initial data
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		rt.Insert(domain, fmt.Sprintf("group%d", i))
	}

	done := make(chan bool)

	// Writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				domain := fmt.Sprintf("concurrent%d-%d.com", id, j)
				rt.Insert(domain, fmt.Sprintf("group%d", id))
			}
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				domain := fmt.Sprintf("domain%d.com", j%100)
				rt.Search(domain)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify
	if group, found := rt.Search("domain0.com"); !found || group != "group0" {
		t.Errorf("After concurrent access, Search(domain0.com) = %q, %v; want group0, true", group, found)
	}
}

// Benchmark tests
func BenchmarkRadixTree_Insert(b *testing.B) {
	rt := NewRadixTree()
	domains := generateDomains(b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Insert(domains[i], "group1")
	}
}

func BenchmarkRadixTree_SearchExact(b *testing.B) {
	rt := NewRadixTree()

	// Insert 10000 exact matches
	domains := generateDomains(10000)
	for _, domain := range domains {
		rt.Insert(domain, "group1")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Search(domains[i%len(domains)])
	}
}

func BenchmarkRadixTree_SearchWildcard(b *testing.B) {
	rt := NewRadixTree()

	// Insert wildcard patterns
	rt.Insert("*.example.com", "group1")
	rt.Insert("*.api.example.com", "group2")
	rt.Insert("*.cn", "group3")

	testDomains := []string{
		"www.example.com",
		"api.example.com",
		"v1.api.example.com",
		"baidu.cn",
		"unknown.org",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Search(testDomains[i%len(testDomains)])
	}
}

func BenchmarkRadixTree_Mixed(b *testing.B) {
	rt := NewRadixTree()

	// Insert mix of exact and wildcard
	for i := 0; i < 5000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		rt.Insert(domain, "group1")
	}
	rt.Insert("*.example.com", "group2")
	rt.Insert("*.cn", "group3")

	testDomains := []string{
		"domain100.com",
		"domain500.com",
		"www.example.com",
		"baidu.cn",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Search(testDomains[i%len(testDomains)])
	}
}

// Comparison benchmarks
func BenchmarkComparison_RadixVsTrie_10K(b *testing.B) {
	domains := generateDomains(10000)

	b.Run("Radix_10K", func(b *testing.B) {
		rt := NewRadixTree()
		for _, domain := range domains {
			rt.Insert(domain, "group1")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.Search(domains[i%len(domains)])
		}
	})

	b.Run("Trie_10K", func(b *testing.B) {
		trie := NewDomainTrie()
		for _, domain := range domains {
			trie.Insert(domain, "group1")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Search(domains[i%len(domains)])
		}
	})

	b.Run("Map_10K", func(b *testing.B) {
		domainMap := make(map[string]string)
		for _, domain := range domains {
			domainMap[domain] = "group1"
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = domainMap[domains[i%len(domains)]]
		}
	})
}

func BenchmarkComparison_RadixVsTrie_100K(b *testing.B) {
	domains := generateDomains(100000)

	b.Run("Radix_100K", func(b *testing.B) {
		rt := NewRadixTree()
		for _, domain := range domains {
			rt.Insert(domain, "group1")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt.Search(domains[i%len(domains)])
		}
	})

	b.Run("Trie_100K", func(b *testing.B) {
		trie := NewDomainTrie()
		for _, domain := range domains {
			trie.Insert(domain, "group1")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Search(domains[i%len(domains)])
		}
	})

	b.Run("Map_100K", func(b *testing.B) {
		domainMap := make(map[string]string)
		for _, domain := range domains {
			domainMap[domain] = "group1"
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = domainMap[domains[i%len(domains)]]
		}
	})
}


func TestRadixTree_InsertBatch(t *testing.T) {
	rt := NewRadixTree()

	// Prepare batch data
	domains := map[string]string{
		"example.com":     "group1",
		"test.com":        "group2",
		"*.example.com":   "wildcard1",
		"*.cn":            "china",
		"specific.com":    "group3",
	}

	// Batch insert
	rt.InsertBatch(domains)

	// Verify all domains were inserted
	tests := []struct {
		domain   string
		expected string
		found    bool
	}{
		{"example.com", "group1", true},
		{"test.com", "group2", true},
		{"www.example.com", "wildcard1", true},
		{"api.example.com", "wildcard1", true},
		{"baidu.cn", "china", true},
		{"specific.com", "group3", true},
		{"unknown.org", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			group, found := rt.Search(tt.domain)
			if found != tt.found {
				t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
			}
			if group != tt.expected {
				t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
			}
		})
	}

	// Verify stats
	stats := rt.Stats()
	if stats.ExactMatches != 3 {
		t.Errorf("ExactMatches = %d, want 3", stats.ExactMatches)
	}
	if stats.WildcardPatterns != 2 {
		t.Errorf("WildcardPatterns = %d, want 2", stats.WildcardPatterns)
	}
}

func TestRadixTree_SearchBatch(t *testing.T) {
	rt := NewRadixTree()

	// Insert test data
	rt.Insert("example.com", "group1")
	rt.Insert("test.com", "group2")
	rt.Insert("*.example.com", "wildcard1")

	// Batch search
	domains := []string{
		"example.com",
		"test.com",
		"www.example.com",
		"unknown.org",
	}

	results := rt.SearchBatch(domains)

	// Verify results
	expected := map[string]string{
		"example.com":     "group1",
		"test.com":        "group2",
		"www.example.com": "wildcard1",
	}

	if len(results) != len(expected) {
		t.Errorf("SearchBatch returned %d results, want %d", len(results), len(expected))
	}

	for domain, expectedGroup := range expected {
		if group, ok := results[domain]; !ok {
			t.Errorf("SearchBatch missing result for %q", domain)
		} else if group != expectedGroup {
			t.Errorf("SearchBatch(%q) = %q, want %q", domain, group, expectedGroup)
		}
	}

	// Verify unknown domain is not in results
	if _, ok := results["unknown.org"]; ok {
		t.Error("SearchBatch should not include unknown.org")
	}
}

func TestRadixTree_StatsCaching(t *testing.T) {
	rt := NewRadixTree()

	// Insert some data
	rt.Insert("example.com", "group1")
	rt.Insert("*.example.com", "wildcard1")

	// First call should calculate stats
	stats1 := rt.Stats()
	if stats1.ExactMatches != 1 || stats1.WildcardPatterns != 1 {
		t.Errorf("Stats1 = %+v, want ExactMatches=1, WildcardPatterns=1", stats1)
	}

	// Second call should use cache (same results)
	stats2 := rt.Stats()
	if stats2 != stats1 {
		t.Errorf("Stats2 = %+v, want %+v (cached)", stats2, stats1)
	}

	// Insert more data
	rt.Insert("test.com", "group2")

	// Stats should be recalculated
	stats3 := rt.Stats()
	if stats3.ExactMatches != 2 {
		t.Errorf("Stats3.ExactMatches = %d, want 2", stats3.ExactMatches)
	}
}

// Benchmark batch operations
func BenchmarkRadixTree_InsertBatch(b *testing.B) {
	domains := make(map[string]string, 1000)
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		domains[domain] = "group1"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := NewRadixTree()
		rt.InsertBatch(domains)
	}
}

func BenchmarkRadixTree_InsertSequential(b *testing.B) {
	domains := make(map[string]string, 1000)
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		domains[domain] = "group1"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := NewRadixTree()
		for domain, group := range domains {
			rt.Insert(domain, group)
		}
	}
}

func BenchmarkRadixTree_SearchBatch(b *testing.B) {
	rt := NewRadixTree()
	
	// Insert test data
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		rt.Insert(domain, "group1")
	}

	// Prepare search list
	searchDomains := make([]string, 100)
	for i := 0; i < 100; i++ {
		searchDomains[i] = fmt.Sprintf("domain%d.com", i*10)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.SearchBatch(searchDomains)
	}
}

func BenchmarkRadixTree_StatsCached(b *testing.B) {
	rt := NewRadixTree()
	
	// Insert test data
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		rt.Insert(domain, "group1")
	}

	// First call to populate cache
	rt.Stats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Stats()
	}
}

func BenchmarkRadixTree_StatsUncached(b *testing.B) {
	rt := NewRadixTree()
	
	// Insert test data
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		rt.Insert(domain, "group1")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Force recalculation by marking dirty
		rt.statsMu.Lock()
		rt.statsDirty = true
		rt.statsMu.Unlock()
		
		rt.Stats()
	}
}
