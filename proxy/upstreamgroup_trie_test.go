package proxy

import (
	"fmt"
	"testing"
)

func TestDomainTrie_BasicOperations(t *testing.T) {
	trie := NewDomainTrie()

	// Test Insert and Search
	t.Run("Insert and Search", func(t *testing.T) {
		trie.Insert("example.com", "group1")
		trie.Insert("www.example.com", "group2")
		trie.Insert("api.example.com", "group3")

		tests := []struct {
			domain   string
			expected string
			found    bool
		}{
			{"example.com", "group1", true},
			{"www.example.com", "group2", true},
			{"api.example.com", "group3", true},
			{"unknown.com", "", false},
		}

		for _, tt := range tests {
			t.Run(tt.domain, func(t *testing.T) {
				group, found := trie.Search(tt.domain)
				if found != tt.found {
					t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
				}
				if group != tt.expected {
					t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
				}
			})
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		trie.Clear()
		trie.Insert("example.com", "group1")
		trie.Insert("www.example.com", "group2")

		// Delete one domain
		trie.Delete("example.com")

		// Should not find deleted domain
		if group, found := trie.Search("example.com"); found {
			t.Errorf("Search after delete found %q, expected not found", group)
		}

		// Should still find other domain
		if group, found := trie.Search("www.example.com"); !found || group != "group2" {
			t.Errorf("Search(www.example.com) = %q, %v; want group2, true", group, found)
		}
	})

	// Test Clear
	t.Run("Clear", func(t *testing.T) {
		trie.Clear()
		trie.Insert("example.com", "group1")
		trie.Insert("www.example.com", "group2")

		trie.Clear()

		if size := trie.Size(); size != 0 {
			t.Errorf("Size after Clear = %d, want 0", size)
		}
	})
}

func TestDomainTrie_WildcardMatching(t *testing.T) {
	trie := NewDomainTrie()

	// Insert wildcard domains
	trie.Insert("*.example.com", "wildcard_group")
	trie.Insert("*.cn", "cn_group")
	trie.Insert("specific.example.com", "specific_group")

	tests := []struct {
		domain   string
		expected string
		found    bool
		desc     string
	}{
		{"www.example.com", "wildcard_group", true, "wildcard match"},
		{"api.example.com", "wildcard_group", true, "wildcard match"},
		{"specific.example.com", "specific_group", true, "exact match overrides wildcard"},
		{"baidu.cn", "cn_group", true, "TLD wildcard"},
		{"www.baidu.cn", "cn_group", true, "TLD wildcard with subdomain"},
		{"example.org", "", false, "no match"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			group, found := trie.Search(tt.domain)
			if found != tt.found {
				t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
			}
			if group != tt.expected {
				t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
			}
		})
	}
}

func TestDomainTrie_TrailingDot(t *testing.T) {
	trie := NewDomainTrie()

	// Insert with and without trailing dot
	trie.Insert("example.com", "group1")
	trie.Insert("www.example.com.", "group2")

	tests := []struct {
		domain   string
		expected string
		found    bool
	}{
		{"example.com", "group1", true},
		{"example.com.", "group1", true},
		{"www.example.com", "group2", true},
		{"www.example.com.", "group2", true},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			group, found := trie.Search(tt.domain)
			if found != tt.found {
				t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
			}
			if group != tt.expected {
				t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
			}
		})
	}
}

func TestDomainTrie_Size(t *testing.T) {
	trie := NewDomainTrie()

	if size := trie.Size(); size != 0 {
		t.Errorf("Initial size = %d, want 0", size)
	}

	trie.Insert("example.com", "group1")
	if size := trie.Size(); size != 1 {
		t.Errorf("Size after 1 insert = %d, want 1", size)
	}

	trie.Insert("www.example.com", "group2")
	trie.Insert("api.example.com", "group3")
	if size := trie.Size(); size != 3 {
		t.Errorf("Size after 3 inserts = %d, want 3", size)
	}

	trie.Delete("example.com")
	if size := trie.Size(); size != 2 {
		t.Errorf("Size after 1 delete = %d, want 2", size)
	}
}

func TestDomainTrie_EmptyDomain(t *testing.T) {
	trie := NewDomainTrie()

	// Insert empty domain should be ignored
	trie.Insert("", "group1")
	if size := trie.Size(); size != 0 {
		t.Errorf("Size after inserting empty domain = %d, want 0", size)
	}

	// Search empty domain should return not found
	if _, found := trie.Search(""); found {
		t.Error("Search empty domain should return not found")
	}
}

func TestDomainTrie_ConcurrentAccess(t *testing.T) {
	trie := NewDomainTrie()

	// Insert some initial data
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("domain%d.com", i)
		trie.Insert(domain, fmt.Sprintf("group%d", i))
	}

	// Concurrent reads and writes
	done := make(chan bool)
	
	// Writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				domain := fmt.Sprintf("concurrent%d-%d.com", id, j)
				trie.Insert(domain, fmt.Sprintf("group%d", id))
			}
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				domain := fmt.Sprintf("domain%d.com", j%100)
				trie.Search(domain)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify some data
	if group, found := trie.Search("domain0.com"); !found || group != "group0" {
		t.Errorf("After concurrent access, Search(domain0.com) = %q, %v; want group0, true", group, found)
	}
}

func TestDomainTrie_ComplexWildcards(t *testing.T) {
	trie := NewDomainTrie()

	// Insert various wildcard patterns
	trie.Insert("*.example.com", "wildcard_example")
	trie.Insert("*.api.example.com", "wildcard_api")
	trie.Insert("specific.api.example.com", "specific_api")
	trie.Insert("*.com", "wildcard_com")

	tests := []struct {
		domain   string
		expected string
		found    bool
		desc     string
	}{
		{"www.example.com", "wildcard_example", true, "first level wildcard"},
		{"v1.api.example.com", "wildcard_api", true, "second level wildcard"},
		{"specific.api.example.com", "specific_api", true, "exact match overrides wildcard"},
		{"test.com", "wildcard_com", true, "TLD wildcard"},
		{"unknown.org", "", false, "no match"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			group, found := trie.Search(tt.domain)
			if found != tt.found {
				t.Errorf("Search(%q) found = %v, want %v", tt.domain, found, tt.found)
			}
			if group != tt.expected {
				t.Errorf("Search(%q) = %q, want %q", tt.domain, group, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkDomainTrie_Insert(b *testing.B) {
	trie := NewDomainTrie()
	domains := generateDomains(b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Insert(domains[i], "group1")
	}
}

func BenchmarkDomainTrie_Search(b *testing.B) {
	trie := NewDomainTrie()
	
	// Insert 10000 domains
	domains := generateDomains(10000)
	for _, domain := range domains {
		trie.Insert(domain, "group1")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Search(domains[i%len(domains)])
	}
}

func BenchmarkDomainTrie_SearchWithWildcards(b *testing.B) {
	trie := NewDomainTrie()
	
	// Insert wildcard patterns
	trie.Insert("*.example.com", "group1")
	trie.Insert("*.api.example.com", "group2")
	trie.Insert("*.cn", "group3")

	testDomains := []string{
		"www.example.com",
		"api.example.com",
		"v1.api.example.com",
		"baidu.cn",
		"unknown.org",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Search(testDomains[i%len(testDomains)])
	}
}

func BenchmarkMap_Search(b *testing.B) {
	// Baseline: map-based search
	domainMap := make(map[string]string)
	
	domains := generateDomains(10000)
	for _, domain := range domains {
		domainMap[domain] = "group1"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = domainMap[domains[i%len(domains)]]
	}
}

// Helper function to generate test domains
func generateDomains(count int) []string {
	domains := make([]string, count)
	for i := 0; i < count; i++ {
		domains[i] = fmt.Sprintf("domain%d.example.com", i)
	}
	return domains
}
