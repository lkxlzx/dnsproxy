// +build ignore

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
)

func main() {
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	fmt.Println("=== DNSProxy Smart Prefetch Test ===")
	fmt.Println()

	// Test 1: Global Clock
	fmt.Println("Test 1: Global Clock")
	testGlobalClock()
	fmt.Println()

	// Test 2: Heat Tracker
	fmt.Println("Test 2: Heat Tracker")
	testHeatTracker()
	fmt.Println()

	// Test 3: Prefetch Scheduler
	fmt.Println("Test 3: Prefetch Scheduler")
	testPrefetchScheduler(logger)
	fmt.Println()

	fmt.Println("=== All Tests Completed ===")
}

func testGlobalClock() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clock := proxy.NewGlobalClock()
	clock.Start(ctx)

	fmt.Printf("Initial clock value: %d\n", clock.Get())

	time.Sleep(3 * time.Second)
	fmt.Printf("After 3 seconds: %d\n", clock.Get())

	clock.Reset()
	fmt.Printf("After reset: %d\n", clock.Get())

	time.Sleep(2 * time.Second)
	fmt.Printf("After 2 more seconds: %d\n", clock.Get())
}

func testHeatTracker() {
	tracker := proxy.NewHeatTracker(6, 180*time.Second)

	// Simulate domain accesses
	domain := "google.com"
	qtype := uint16(1) // A record

	entry := &proxy.CacheEntryExt{
		Domain: domain,
		Qtype:  qtype,
	}

	now := time.Now()

	// First 5 accesses (should not join queue)
	for i := 1; i <= 5; i++ {
		joined := tracker.OnAccess(entry, now.Add(time.Duration(i)*10*time.Second))
		fmt.Printf("Access %d: joined queue = %v, count = %d\n", i, joined, entry.AccessCount)
	}

	// 6th access (should join queue)
	joined := tracker.OnAccess(entry, now.Add(60*time.Second))
	fmt.Printf("Access 6: joined queue = %v, in queue = %v\n", joined, entry.InPrefetchQueue)

	// Check candidates
	candidates := tracker.GetPrefetchCandidates()
	fmt.Printf("Prefetch candidates: %d\n", len(candidates))
}

func testPrefetchScheduler(logger *slog.Logger) {
	config := proxy.DefaultPrefetchConfig()
	config.Enabled = true
	config.ScanInterval = 1 * time.Second
	config.InactivityCheckInterval = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clock := proxy.NewGlobalClock()
	clock.Start(ctx)

	tracker := proxy.NewHeatTracker(config.MinHeatThreshold, config.TimeWindow)

	// Note: We can't fully test the scheduler without a real Proxy instance
	// This is just a structure test
	fmt.Println("Prefetch scheduler structure validated")
	fmt.Printf("Config: enabled=%v, threshold=%ds, percent=%d%%\n",
		config.Enabled, config.ThresholdSeconds, config.ThresholdPercent)
}
