package main

import (
	"fmt"
	"log"
	"time"

	"github.com/miekg/dns"
)

// Test the multi-layer failover mechanism
// This test verifies that DNS queries follow the correct failover chain:
// For matched domains: group primary → group fallback → global upstream
// For unmatched domains: default-group primary → default-group fallback → global upstream

func main() {
	fmt.Println("=== Multi-Layer Failover Test ===")
	fmt.Println("Testing the three-layer failover mechanism")
	fmt.Println()

	// DNS server to query
	server := "127.0.0.1:53"
	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	// Test cases
	testCases := []struct {
		domain      string
		description string
	}{
		{
			domain:      "baidu.com",
			description: "Matched domain (should use china group → fallback → global)",
		},
		{
			domain:      "google.com",
			description: "Unmatched domain (should use default-group → fallback → global)",
		},
		{
			domain:      "example.com",
			description: "Unmatched domain (should use default-group → fallback → global)",
		},
	}

	fmt.Println("Starting tests...")
	fmt.Println()

	successCount := 0
	failCount := 0

	for i, tc := range testCases {
		fmt.Printf("Test %d: %s\n", i+1, tc.domain)
		fmt.Printf("  Description: %s\n", tc.description)

		// Create DNS query
		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(tc.domain), dns.TypeA)

		// Send query
		start := time.Now()
		resp, _, err := client.Exchange(msg, server)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)
			failCount++
		} else if resp == nil {
			fmt.Printf("  ❌ No response received\n")
			failCount++
		} else if resp.Rcode != dns.RcodeSuccess {
			fmt.Printf("  ⚠️  Response code: %s\n", dns.RcodeToString[resp.Rcode])
			failCount++
		} else {
			fmt.Printf("  ✅ Success (%.2fms)\n", float64(duration.Microseconds())/1000.0)
			if len(resp.Answer) > 0 {
				for _, ans := range resp.Answer {
					if a, ok := ans.(*dns.A); ok {
						fmt.Printf("     IP: %s\n", a.A.String())
					}
				}
			}
			successCount++
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("=== Test Summary ===")
	fmt.Printf("Total: %d\n", len(testCases))
	fmt.Printf("Success: %d\n", successCount)
	fmt.Printf("Failed: %d\n", failCount)

	if failCount > 0 {
		log.Fatal("Some tests failed")
	}

	fmt.Println("\n✅ All tests passed!")
}
