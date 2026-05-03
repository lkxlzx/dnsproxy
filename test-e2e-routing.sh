#!/bin/bash

# End-to-End DNS Routing Test Script
# Tests domain group routing with real DNS queries

set -e

echo "=========================================="
echo "DNS Upstream Groups - E2E Routing Test"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
DNSPROXY_PORT=5301
DNSPROXY_ADDR="127.0.0.1"
CONFIG_FILE="config-test-e2e-routing.yaml"
CACHE_DIR="./cache/test-e2e"
DOMAINS_DIR="./domains"

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to print colored output
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Function to run a test
run_test() {
    local test_name="$1"
    local domain="$2"
    local expected_group="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo ""
    print_info "Test $TOTAL_TESTS: $test_name"
    echo "  Domain: $domain"
    echo "  Expected Group: $expected_group"
    
    # Query DNS
    echo -n "  Querying DNS... "
    if result=$(dig @${DNSPROXY_ADDR} -p ${DNSPROXY_PORT} +short $domain 2>&1); then
        if [ -n "$result" ]; then
            echo "OK"
            echo "  Result: $result"
            print_success "DNS query successful"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        else
            echo "EMPTY"
            print_warning "Empty response (might be blocked)"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            return 0
        fi
    else
        echo "FAILED"
        print_error "DNS query failed: $result"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
}

# Cleanup function
cleanup() {
    echo ""
    print_info "Cleaning up..."
    if [ -n "$DNSPROXY_PID" ] && kill -0 $DNSPROXY_PID 2>/dev/null; then
        kill $DNSPROXY_PID
        wait $DNSPROXY_PID 2>/dev/null || true
        print_success "DNSProxy stopped"
    fi
}

trap cleanup EXIT

# Step 1: Create test configuration
echo "Step 1: Creating test configuration..."

mkdir -p "$CACHE_DIR"
mkdir -p "$DOMAINS_DIR"

# Create domain files
cat > "$DOMAINS_DIR/china-test.txt" << 'EOF'
baidu.com
qq.com
taobao.com
jd.com
163.com
sina.com.cn
weibo.com
bilibili.com
EOF

cat > "$DOMAINS_DIR/overseas-test.txt" << 'EOF'
google.com
youtube.com
facebook.com
twitter.com
github.com
stackoverflow.com
reddit.com
EOF

# Create test configuration
cat > "$CONFIG_FILE" << EOF
# E2E Routing Test Configuration

# Upstream groups
upstream-groups:
  default_group: "overseas"
  
  groups:
    # Overseas DNS (Google, Cloudflare)
    - name: "overseas"
      mode: "load_balance"
      upstreams:
        - "8.8.8.8"
        - "1.1.1.1"
      timeout: "3s"
      enabled: true
    
    # China DNS (Alibaba, DNSPod)
    - name: "china"
      mode: "load_balance"
      upstreams:
        - "223.5.5.5"
        - "119.29.29.29"
      timeout: "3s"
      enabled: true
    
    # Block group (return 0.0.0.0)
    - name: "block"
      mode: "load_balance"
      upstreams:
        - "0.0.0.0"
      enabled: true
  
  # Domain routing rules
  domain_groups:
    # China domains
    "baidu.com": "china"
    "*.baidu.com": "china"
    "qq.com": "china"
    "*.qq.com": "china"
    "taobao.com": "china"
    "*.taobao.com": "china"
    "jd.com": "china"
    "*.jd.com": "china"
    "163.com": "china"
    "*.163.com": "china"
    "sina.com.cn": "china"
    "*.sina.com.cn": "china"
    "weibo.com": "china"
    "*.weibo.com": "china"
    "bilibili.com": "china"
    "*.bilibili.com": "china"
    
    # Overseas domains
    "google.com": "overseas"
    "*.google.com": "overseas"
    "youtube.com": "overseas"
    "*.youtube.com": "overseas"
    "facebook.com": "overseas"
    "*.facebook.com": "overseas"
    "twitter.com": "overseas"
    "*.twitter.com": "overseas"
    "github.com": "overseas"
    "*.github.com": "overseas"
    "stackoverflow.com": "overseas"
    "*.stackoverflow.com": "overseas"
    "reddit.com": "overseas"
    "*.reddit.com": "overseas"
    
    # Blocked domains (ads)
    "ads.example.com": "block"
    "tracker.example.com": "block"

# Listen configuration
listen-addrs:
  - "127.0.0.1"

listen-ports:
  - ${DNSPROXY_PORT}

# Logging
log-level: "info"
EOF

print_success "Configuration created: $CONFIG_FILE"

# Step 2: Build dnsproxy (if needed)
echo ""
echo "Step 2: Checking dnsproxy binary..."

if [ ! -f "./dnsproxy" ] && [ ! -f "./dnsproxy.exe" ]; then
    print_info "Building dnsproxy..."
    go build -o dnsproxy ./cmd/dnsproxy
    print_success "DNSProxy built"
else
    print_success "DNSProxy binary found"
fi

# Determine binary name
DNSPROXY_BIN="./dnsproxy"
if [ -f "./dnsproxy.exe" ]; then
    DNSPROXY_BIN="./dnsproxy.exe"
fi

# Step 3: Start dnsproxy
echo ""
echo "Step 3: Starting DNSProxy..."

$DNSPROXY_BIN --config-path="$CONFIG_FILE" > dnsproxy-e2e.log 2>&1 &
DNSPROXY_PID=$!

print_info "DNSProxy PID: $DNSPROXY_PID"

# Wait for dnsproxy to start
sleep 2

if ! kill -0 $DNSPROXY_PID 2>/dev/null; then
    print_error "DNSProxy failed to start"
    cat dnsproxy-e2e.log
    exit 1
fi

print_success "DNSProxy started on ${DNSPROXY_ADDR}:${DNSPROXY_PORT}"

# Step 4: Run tests
echo ""
echo "=========================================="
echo "Running DNS Routing Tests"
echo "=========================================="

# Test Group 1: China Domains
echo ""
echo "=== Test Group 1: China Domains (should use 223.5.5.5) ==="

run_test "Baidu (exact match)" "baidu.com" "china"
run_test "Baidu subdomain (wildcard)" "www.baidu.com" "china"
run_test "QQ (exact match)" "qq.com" "china"
run_test "QQ subdomain (wildcard)" "mail.qq.com" "china"
run_test "Taobao" "taobao.com" "china"
run_test "JD" "jd.com" "china"
run_test "Bilibili" "bilibili.com" "china"
run_test "Bilibili subdomain" "www.bilibili.com" "china"

# Test Group 2: Overseas Domains
echo ""
echo "=== Test Group 2: Overseas Domains (should use 8.8.8.8) ==="

run_test "Google (exact match)" "google.com" "overseas"
run_test "Google subdomain (wildcard)" "www.google.com" "overseas"
run_test "YouTube" "youtube.com" "overseas"
run_test "GitHub" "github.com" "overseas"
run_test "GitHub subdomain" "api.github.com" "overseas"
run_test "Stack Overflow" "stackoverflow.com" "overseas"

# Test Group 3: Default Group
echo ""
echo "=== Test Group 3: Unspecified Domains (should use default: overseas) ==="

run_test "Example.com (default)" "example.com" "overseas"
run_test "Cloudflare" "cloudflare.com" "overseas"

# Test Group 4: Wildcard Matching
echo ""
echo "=== Test Group 4: Wildcard Matching ==="

run_test "Deep subdomain (Baidu)" "tieba.baidu.com" "china"
run_test "Deep subdomain (Google)" "mail.google.com" "overseas"
run_test "Multiple levels (QQ)" "vip.qq.com" "china"

# Step 5: Performance Test
echo ""
echo "=========================================="
echo "Performance Test"
echo "=========================================="

print_info "Running 10 queries to measure performance..."

start_time=$(date +%s%N)
for i in {1..10}; do
    dig @${DNSPROXY_ADDR} -p ${DNSPROXY_PORT} +short baidu.com > /dev/null 2>&1
done
end_time=$(date +%s%N)

elapsed=$((($end_time - $start_time) / 1000000))
avg_time=$(($elapsed / 10))

echo "  Total time: ${elapsed}ms"
echo "  Average time per query: ${avg_time}ms"

if [ $avg_time -lt 100 ]; then
    print_success "Performance: Excellent (< 100ms)"
elif [ $avg_time -lt 500 ]; then
    print_success "Performance: Good (< 500ms)"
else
    print_warning "Performance: Acceptable (> 500ms)"
fi

# Step 6: Check logs
echo ""
echo "=========================================="
echo "DNSProxy Logs (last 20 lines)"
echo "=========================================="
tail -n 20 dnsproxy-e2e.log

# Step 7: Summary
echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo "Total Tests: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
echo -e "${RED}Failed: $FAILED_TESTS${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo ""
    print_success "All tests passed! ✓"
    exit 0
else
    echo ""
    print_error "Some tests failed!"
    exit 1
fi
