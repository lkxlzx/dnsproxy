package proxy

import (
	"fmt"
	"testing"
)

// BenchmarkComparison_SmallDataset compares Trie vs Map performance with 100 domains
func BenchmarkComparison_SmallDataset(b *testing.B) {
	domains := generateDomains(100)
	
	b.Run("Trie_100domains", func(b *testing.B) {
		trie := NewDomainTrie()
		for _, domain := range domains {
			trie.Insert(domain, "group1")
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Search(domains[i%len(domains)])
		}
	})
	
	b.Run("Map_100domains", func(b *testing.B) {
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

// BenchmarkComparison_MediumDataset compares Trie vs Map performance with 10,000 domains
func BenchmarkComparison_MediumDataset(b *testing.B) {
	domains := generateDomains(10000)
	
	b.Run("Trie_10Kdomains", func(b *testing.B) {
		trie := NewDomainTrie()
		for _, domain := range domains {
			trie.Insert(domain, "group1")
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Search(domains[i%len(domains)])
		}
	})
	
	b.Run("Map_10Kdomains", func(b *testing.B) {
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

// BenchmarkComparison_LargeDataset compares Trie vs Map performance with 100,000 domains
func BenchmarkComparison_LargeDataset(b *testing.B) {
	domains := generateDomains(100000)
	
	b.Run("Trie_100Kdomains", func(b *testing.B) {
		trie := NewDomainTrie()
		for _, domain := range domains {
			trie.Insert(domain, "group1")
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			trie.Search(domains[i%len(domains)])
		}
	})
	
	b.Run("Map_100Kdomains", func(b *testing.B) {
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

// BenchmarkWildcardMatching compares wildcard matching performance
func BenchmarkWildcardMatching(b *testing.B) {
	// Setup: Insert wildcard patterns
	trie := NewDomainTrie()
	trie.Insert("*.example.com", "group1")
	trie.Insert("*.api.example.com", "group2")
	trie.Insert("*.cn", "group3")
	trie.Insert("*.com", "group4")
	
	testDomains := []string{
		"www.example.com",
		"api.example.com",
		"v1.api.example.com",
		"test.example.com",
		"baidu.cn",
		"qq.cn",
		"google.com",
		"github.com",
	}
	
	b.Run("Trie_Wildcard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			trie.Search(testDomains[i%len(testDomains)])
		}
	})
	
	// Map-based wildcard matching (simulating the old implementation)
	domainMap := map[string]string{
		"*.example.com":     "group1",
		"*.api.example.com": "group2",
		"*.cn":              "group3",
		"*.com":             "group4",
	}
	
	b.Run("Map_Wildcard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			domain := testDomains[i%len(testDomains)]
			
			// Try exact match
			if _, ok := domainMap[domain]; ok {
				continue
			}
			
			// Try wildcard matching (simplified version)
			labels := splitDomain(domain)
			for j := 1; j < len(labels); j++ {
				wildcard := "*." + joinLabels(labels[j:])
				if _, ok := domainMap[wildcard]; ok {
					break
				}
			}
		}
	})
}

// BenchmarkRealWorldScenario simulates a real-world DNS proxy scenario
func BenchmarkRealWorldScenario(b *testing.B) {
	// Simulate a real-world scenario with:
	// - 50,000 Chinese domains
	// - 30,000 overseas domains
	// - 10,000 ad domains
	// - Wildcard patterns
	
	trie := NewDomainTrie()
	
	// Insert Chinese domains
	for i := 0; i < 50000; i++ {
		domain := fmt.Sprintf("cn-domain%d.cn", i)
		trie.Insert(domain, "china")
	}
	
	// Insert overseas domains
	for i := 0; i < 30000; i++ {
		domain := fmt.Sprintf("overseas-domain%d.com", i)
		trie.Insert(domain, "overseas")
	}
	
	// Insert ad domains
	for i := 0; i < 10000; i++ {
		domain := fmt.Sprintf("ad-domain%d.com", i)
		trie.Insert(domain, "ads")
	}
	
	// Insert wildcard patterns
	trie.Insert("*.baidu.com", "china")
	trie.Insert("*.qq.com", "china")
	trie.Insert("*.google.com", "overseas")
	trie.Insert("*.facebook.com", "overseas")
	
	// Test domains (mix of exact and wildcard matches)
	testDomains := []string{
		"cn-domain12345.cn",
		"overseas-domain5678.com",
		"ad-domain999.com",
		"www.baidu.com",
		"mail.qq.com",
		"www.google.com",
		"api.facebook.com",
		"unknown-domain.org",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trie.Search(testDomains[i%len(testDomains)])
	}
}

// BenchmarkMemoryUsage measures memory usage of Trie vs Map
func BenchmarkMemoryUsage(b *testing.B) {
	domains := generateDomains(10000)
	
	b.Run("Trie_Memory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			trie := NewDomainTrie()
			for _, domain := range domains {
				trie.Insert(domain, "group1")
			}
		}
	})
	
	b.Run("Map_Memory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			domainMap := make(map[string]string)
			for _, domain := range domains {
				domainMap[domain] = "group1"
			}
		}
	})
}

// Helper functions
func splitDomain(domain string) []string {
	labels := []string{}
	start := 0
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' {
			labels = append(labels, domain[start:i])
			start = i + 1
		}
	}
	if start < len(domain) {
		labels = append(labels, domain[start:])
	}
	return labels
}

func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	result := labels[0]
	for i := 1; i < len(labels); i++ {
		result += "." + labels[i]
	}
	return result
}
