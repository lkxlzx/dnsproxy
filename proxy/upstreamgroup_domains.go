package proxy

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DomainFileLoader loads domain lists from files or URLs.
type DomainFileLoader struct {
	logger             *slog.Logger
	httpClient         *http.Client
	lastDetectedFormat string // Store the last detected format
}

// NewDomainFileLoader creates a new domain file loader.
func NewDomainFileLoader(logger *slog.Logger) *DomainFileLoader {
	if logger == nil {
		logger = slog.Default()
	}

	return &DomainFileLoader{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Increased timeout for large files
			Transport: &http.Transport{
				MaxIdleConns:          10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    false,
			},
		},
	}
}

// LoadDomains loads domains from a file or URL.
// Supports multiple formats:
// - Plain text (one domain per line)
// - YAML (Clash format)
// - GFWList (base64 encoded)
func (dfl *DomainFileLoader) LoadDomains(source string) ([]string, error) {
	dfl.logger.Debug("loading domains", "source", source)

	var content []byte
	var err error

	// Check if source is a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		content, err = dfl.loadFromURL(source)
	} else {
		content, err = dfl.loadFromFile(source)
	}

	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}

	// Detect format and parse
	domains, err := dfl.parseDomains(content, source)
	if err != nil {
		return nil, fmt.Errorf("parse domains: %w", err)
	}

	dfl.logger.Info("domains loaded", "source", source, "count", len(domains))
	return domains, nil
}

// loadFromFile loads content from a local file.
func (dfl *DomainFileLoader) loadFromFile(path string) ([]byte, error) {
	dfl.logger.Debug("loading from file", "path", path)
	return os.ReadFile(path)
}

// loadFromURL loads content from a URL.
func (dfl *DomainFileLoader) loadFromURL(url string) ([]byte, error) {
	dfl.logger.Debug("loading from URL", "url", url)

	resp, err := dfl.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// parseDomains parses domains from content based on format detection.
func (dfl *DomainFileLoader) parseDomains(content []byte, source string) ([]string, error) {
	// Try to detect format
	contentStr := string(content)

	// Detect and parse format
	format := dfl.detectFormat(contentStr, source)
	dfl.lastDetectedFormat = format // Store detected format
	dfl.logger.Debug("detected format", "format", format, "source", source)

	var domains []string
	var err error

	switch format {
	case "clash":
		domains, err = dfl.parseClashYAML(content)
	case "gfwlist":
		domains, err = dfl.parseGFWList(content)
	case "surge":
		domains, err = dfl.parseSurge(content)
	case "dnsmasq":
		domains, err = dfl.parseDnsmasq(content)
	case "hosts":
		domains, err = dfl.parseHosts(content)
	case "adblock":
		domains, err = dfl.parseAdblock(content)
	case "json":
		domains, err = dfl.parseJSON(content)
	default:
		domains, err = dfl.parsePlainText(content)
	}

	if err != nil {
		return nil, fmt.Errorf("parse %s format: %w", format, err)
	}

	// Post-process: clean and deduplicate
	domains = dfl.cleanAndDeduplicate(domains)

	return domains, nil
}

// detectFormat detects the file format.
func (dfl *DomainFileLoader) detectFormat(content, source string) string {
	// Check file extension
	if strings.HasSuffix(source, ".yaml") || strings.HasSuffix(source, ".yml") {
		if strings.Contains(content, "payload:") {
			return "clash"
		}
		return "plain"
	}

	if strings.HasSuffix(source, ".json") {
		return "json"
	}

	if strings.HasSuffix(source, ".conf") {
		if strings.Contains(content, "DOMAIN-SUFFIX") {
			return "surge"
		}
		if strings.Contains(content, "server=/") {
			return "dnsmasq"
		}
	}

	// Check content patterns
	if strings.Contains(content, "payload:") && strings.Contains(content, "DOMAIN") {
		return "clash"
	}

	if strings.Contains(source, "gfwlist") || strings.HasPrefix(content, "[AutoProxy") {
		return "gfwlist"
	}

	if strings.Contains(content, "server=/") || strings.Contains(content, "address=/") {
		return "dnsmasq"
	}

	if strings.Contains(content, "DOMAIN-SUFFIX,") || strings.Contains(content, "DOMAIN,") {
		return "surge"
	}

	// Check if it's hosts format (IP domain)
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if firstLine != "" && !strings.HasPrefix(firstLine, "#") {
			parts := strings.Fields(firstLine)
			if len(parts) >= 2 && dfl.isIPAddress(parts[0]) {
				return "hosts"
			}
		}
	}

	// Check if it's adblock format
	if strings.Contains(content, "||") || strings.Contains(content, "@@||") {
		return "adblock"
	}

	return "plain"
}

// parsePlainText parses plain text format (one domain per line).
func (dfl *DomainFileLoader) parsePlainText(content []byte) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		// Clean up the domain
		domain := dfl.cleanDomain(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan lines: %w", err)
	}

	return domains, nil
}

// parseClashYAML parses Clash YAML format.
// Example format:
// payload:
//   - DOMAIN,example.com
//   - DOMAIN-SUFFIX,google.com
//   - DOMAIN-KEYWORD,youtube
func (dfl *DomainFileLoader) parseClashYAML(content []byte) ([]string, error) {
	var data struct {
		Payload []string `yaml:"payload"`
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	var domains []string
	for _, rule := range data.Payload {
		domain := dfl.parseClashRule(rule)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	return domains, nil
}

// parseClashRule parses a single Clash rule.
func (dfl *DomainFileLoader) parseClashRule(rule string) string {
	parts := strings.Split(rule, ",")
	if len(parts) < 2 {
		return ""
	}

	ruleType := strings.TrimSpace(parts[0])
	domain := strings.TrimSpace(parts[1])

	switch ruleType {
	case "DOMAIN":
		// Exact domain match (no trailing dot)
		return domain
	case "DOMAIN-SUFFIX":
		// Suffix match (wildcard, no trailing dot)
		return "*." + domain
	case "DOMAIN-KEYWORD":
		// Keyword match (not directly supported, treat as suffix, no trailing dot)
		return "*" + domain + "*"
	default:
		// Skip other rule types (IP-CIDR, etc.)
		return ""
	}
}

// parseGFWList parses GFWList format (base64 encoded).
func (dfl *DomainFileLoader) parseGFWList(content []byte) ([]string, error) {
	contentStr := string(content)

	// Remove header if present
	if strings.HasPrefix(contentStr, "[AutoProxy") {
		lines := strings.Split(contentStr, "\n")
		if len(lines) > 1 {
			contentStr = strings.Join(lines[1:], "\n")
		}
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(contentStr)
	if err != nil {
		// If decode fails, try to parse as plain text
		dfl.logger.Warn("base64 decode failed, trying plain text", "error", err)
		return dfl.parsePlainText(content)
	}

	// Parse decoded content
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(decoded)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		// Parse GFWList rules
		domain := dfl.parseGFWListRule(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan lines: %w", err)
	}

	return domains, nil
}

// parseGFWListRule parses a single GFWList rule.
func (dfl *DomainFileLoader) parseGFWListRule(rule string) string {
	// Remove common prefixes
	rule = strings.TrimPrefix(rule, "||")
	rule = strings.TrimPrefix(rule, "|")
	rule = strings.TrimPrefix(rule, ".")

	// Remove common suffixes
	rule = strings.TrimSuffix(rule, "^")
	rule = strings.TrimSuffix(rule, "/")

	// Skip rules with wildcards in the middle
	if strings.Contains(rule, "*") && !strings.HasPrefix(rule, "*") {
		return ""
	}

	// Skip regex rules
	if strings.HasPrefix(rule, "/") || strings.Contains(rule, "(") {
		return ""
	}

	// Extract domain from URL
	if strings.Contains(rule, "://") {
		parts := strings.Split(rule, "://")
		if len(parts) > 1 {
			rule = parts[1]
		}
	}

	// Get domain part (before path)
	if idx := strings.Index(rule, "/"); idx > 0 {
		rule = rule[:idx]
	}

	// Clean and validate (no trailing dot)
	domain := dfl.cleanDomain(rule)
	return domain
}

// isValidIPv4 checks if a string is a valid IPv4 address.
func (dfl *DomainFileLoader) isValidIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	
	for _, part := range parts {
		// Empty part or too long
		if part == "" || len(part) > 3 {
			return false
		}
		
		// Check if all characters are digits and value is 0-255
		num := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
			num = num*10 + int(ch-'0')
		}
		
		// Check range
		if num > 255 {
			return false
		}
		
		// Leading zeros are not allowed (except "0" itself)
		if len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	
	return true
}

// cleanDomain cleans and validates a domain name.
func (dfl *DomainFileLoader) cleanDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)

	// Remove protocol
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")

	// Remove port
	if idx := strings.Index(domain, ":"); idx > 0 {
		domain = domain[:idx]
	}

	// Remove path
	if idx := strings.Index(domain, "/"); idx > 0 {
		domain = domain[:idx]
	}

	// Skip if empty or contains spaces
	if domain == "" || strings.Contains(domain, " ") {
		return ""
	}

	// Skip IP addresses (use improved validation)
	if dfl.isValidIPv4(domain) {
		return ""
	}

	return domain
}

// isFileOrURLSource checks if a string is a file path or URL.
// Returns true for:
//   - URLs: http://, https://
//   - Absolute paths: / (Unix), C:\ or C:/ (Windows)
//   - Relative paths: ./, ../
func isFileOrURLSource(s string) bool {
	// URL
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return true
	}
	
	// Absolute path (Unix)
	if strings.HasPrefix(s, "/") {
		return true
	}
	
	// Relative path
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	
	// Windows absolute path (C:\ or C:/)
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	
	return false
}

// LoadDomainsFromConfig loads domains from domain_groups configuration.
// It processes file references and expands them into domain lists.
// Supports both simple string format and object format with refresh_interval.
//
// Configuration format:
//   domain_groups:
//     "*.google.com": "overseas"           # domain pattern -> group name
//     "china": "./domains/china.txt"       # group name -> file path
//     "overseas":                          # group name -> array of sources
//       - "google.com"
//       - source: "https://example.com/list.txt"
//         refresh_interval: "6h"
//         enabled: true
func LoadDomainsFromConfig(
	domainGroups map[string]interface{},
	logger *slog.Logger,
	manager *DomainListManager,
) (map[string]string, error) {
	if logger == nil {
		logger = slog.Default()
	}

	loader := NewDomainFileLoader(logger)
	result := make(map[string]string)

	for key, value := range domainGroups {
		// Parse key format: could be "name", "id", or "name/id"
		// Extract the group reference (prefer ID if present)
		groupRef := parseGroupReference(key)
		
		logger.Debug("processing domain group",
			"key", key,
			"groupRef", groupRef)

		// Handle different value types
		switch v := value.(type) {
		case string:
			// Simple string format: could be domain->group or group->file
			// Check if value is a file/URL (then key is group name)
			// Otherwise it's domain->group mapping
			if isFileOrURLSource(v) {
				// key is group name/id, value is file/URL
				if err := loadDomainsFromSource(v, groupRef, loader, result, logger); err != nil {
					return nil, err
				}
			} else {
				// key is domain pattern, value is group name/id
				// Parse the value as well (it might be "name/id" format)
				valueRef := parseGroupReference(v)
				result[key] = valueRef
			}

		case []interface{}:
			// Array format: key is group name, values are sources or domains
			for i, item := range v {
				switch itemVal := item.(type) {
				case string:
					// String in array
					if isFileOrURLSource(itemVal) {
						if err := loadDomainsFromSource(itemVal, groupRef, loader, result, logger); err != nil {
							return nil, fmt.Errorf("processing array item %d: %w", i, err)
						}
					} else {
						// Direct domain mapping
						result[itemVal] = groupRef
					}

				case map[string]interface{}:
					// Object in array with refresh_interval
					if err := processObjectSource(itemVal, groupRef, loader, result, manager, logger); err != nil {
						return nil, fmt.Errorf("processing array item %d: %w", i, err)
					}

				default:
					return nil, fmt.Errorf("unsupported array item type for group %q at index %d", key, i)
				}
			}

		case map[string]interface{}:
			// Object format: could have "domains" field (AdGuard Home format)
			// or be a source object with refresh_interval
			if domainsVal, hasDomains := v["domains"]; hasDomains {
				// AdGuard Home format: { domains: [...] }
				if err := processDomainsField(domainsVal, groupRef, loader, result, manager, logger); err != nil {
					return nil, fmt.Errorf("processing domains field for %q: %w", key, err)
				}
			} else {
				// Single object format with source/refresh_interval
				if err := processObjectSource(v, groupRef, loader, result, manager, logger); err != nil {
					return nil, err
				}
			}

		default:
			return nil, fmt.Errorf("unsupported value type for group %q", key)
		}
	}

	return result, nil
}

// parseGroupReference extracts group reference from key.
// The key can be a group name or ID directly.
func parseGroupReference(key string) string {
	// Simply return the key as-is
	// It will be resolved to a name later using the idToName map
	return key
}

// processDomainsField processes the "domains" field from AdGuard Home format.
func processDomainsField(
	domainsVal interface{},
	groupRef string,
	loader *DomainFileLoader,
	result map[string]string,
	manager *DomainListManager,
	logger *slog.Logger,
) error {
	switch domains := domainsVal.(type) {
	case []interface{}:
		// Array of domains or sources
		for i, item := range domains {
			switch itemVal := item.(type) {
			case string:
				if isFileOrURLSource(itemVal) {
					// Load from file/URL
					if err := loadDomainsFromSource(itemVal, groupRef, loader, result, logger); err != nil {
						return fmt.Errorf("loading domain source at index %d: %w", i, err)
					}
				} else {
					// Direct domain mapping
					result[itemVal] = groupRef
				}
			
			case map[string]interface{}:
				// Object with source and refresh_interval
				if err := processObjectSource(itemVal, groupRef, loader, result, manager, logger); err != nil {
					return fmt.Errorf("processing domain object at index %d: %w", i, err)
				}
			
			default:
				return fmt.Errorf("unsupported domain item type at index %d", i)
			}
		}
		return nil
	
	default:
		return fmt.Errorf("domains field must be an array")
	}
}

// loadDomainsFromSource loads domains from a file or URL and maps them to a group.
func loadDomainsFromSource(
	source string,
	groupName string,
	loader *DomainFileLoader,
	result map[string]string,
	logger *slog.Logger,
) error {
	logger.Info("loading domains from file", "group", groupName, "source", source)

	// Load domains from file
	domains, err := loader.LoadDomains(source)
	if err != nil {
		logger.Error("failed to load domains", "source", source, "error", err)
		return fmt.Errorf("load domains from %q: %w", source, err)
	}

	// Add all loaded domains to result
	for _, domain := range domains {
		result[domain] = groupName
	}

	logger.Info("domains loaded from file",
		"source", source,
		"group", groupName,
		"count", len(domains))

	return nil
}

// processObjectSource processes an object source with custom refresh_interval.
func processObjectSource(
	obj map[string]interface{},
	groupName string,
	loader *DomainFileLoader,
	result map[string]string,
	manager *DomainListManager,
	logger *slog.Logger,
) error {
	// Extract source
	sourceVal, ok := obj["source"]
	if !ok {
		return fmt.Errorf("missing 'source' field in object for group %q", groupName)
	}

	source, ok := sourceVal.(string)
	if !ok {
		return fmt.Errorf("'source' must be a string for group %q", groupName)
	}

	// Extract enabled (default: true)
	enabled := true
	if enabledVal, ok := obj["enabled"]; ok {
		if enabledBool, ok := enabledVal.(bool); ok {
			enabled = enabledBool
		}
	}

	// Skip if disabled
	if !enabled {
		logger.Info("skipping disabled source", "group", groupName, "source", source)
		return nil
	}

	// Extract refresh_interval (optional)
	var refreshInterval time.Duration
	if intervalVal, ok := obj["refresh_interval"]; ok {
		if intervalStr, ok := intervalVal.(string); ok {
			var err error
			refreshInterval, err = time.ParseDuration(intervalStr)
			if err != nil {
				return fmt.Errorf("invalid refresh_interval for group %q: %w", groupName, err)
			}
		}
	}

	logger.Info("loading domains from source with custom interval",
		"group", groupName,
		"source", source,
		"refresh_interval", refreshInterval)

	// Load domains
	domains, err := loader.LoadDomains(source)
	if err != nil {
		logger.Error("failed to load domains", "source", source, "error", err)
		return fmt.Errorf("load domains from %q: %w", source, err)
	}

	// Add all loaded domains to result
	for _, domain := range domains {
		result[domain] = groupName
	}

	// Register with manager if provided
	if manager != nil {
		managedList := &ManagedList{
			Name:            fmt.Sprintf("%s_%s", groupName, hashSource(source)),
			Source:          source,
			Group:           groupName,
			Enabled:         enabled,
			LastUpdate:      time.Now(),
			DomainCount:     len(domains),
			AutoUpdate:      true,
			RefreshInterval: refreshInterval,
		}

		if err := manager.AddList(managedList); err != nil {
			// Log but don't fail - list might already exist
			logger.Warn("failed to add managed list", "name", managedList.Name, "error", err)
		}
	}

	logger.Info("domains loaded from source",
		"source", source,
		"group", groupName,
		"count", len(domains),
		"refresh_interval", refreshInterval)

	return nil
}

// hashSource creates a simple hash of the source URL for unique naming.
func hashSource(source string) string {
	// Simple hash: take last part of URL or first 8 chars
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 8 {
			return last[:8]
		}
		return last
	}
	if len(source) > 8 {
		return source[:8]
	}
	return source
}


// parseSurge parses Surge rule format.
// Example: DOMAIN-SUFFIX,google.com
func (dfl *DomainFileLoader) parseSurge(content []byte) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Parse Surge rules
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		ruleType := strings.TrimSpace(parts[0])
		domain := strings.TrimSpace(parts[1])

		switch ruleType {
		case "DOMAIN":
			domains = append(domains, domain)
		case "DOMAIN-SUFFIX":
			domains = append(domains, "*."+domain)
		case "DOMAIN-KEYWORD":
			domains = append(domains, "*"+domain+"*")
		}
	}

	return domains, scanner.Err()
}

// parseDnsmasq parses dnsmasq configuration format.
// Example: server=/google.com/8.8.8.8
//          address=/ads.com/0.0.0.0
func (dfl *DomainFileLoader) parseDnsmasq(content []byte) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse server= format
		if strings.HasPrefix(line, "server=/") {
			parts := strings.Split(line, "/")
			if len(parts) >= 2 {
				domain := parts[1]
				if domain != "" {
					domains = append(domains, domain)
				}
			}
		}

		// Parse address= format
		if strings.HasPrefix(line, "address=/") {
			parts := strings.Split(line, "/")
			if len(parts) >= 2 {
				domain := parts[1]
				if domain != "" {
					domains = append(domains, domain)
				}
			}
		}
	}

	return domains, scanner.Err()
}

// parseHosts parses hosts file format.
// Example: 127.0.0.1 localhost
//          0.0.0.0 ads.example.com
func (dfl *DomainFileLoader) parseHosts(content []byte) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse hosts format: IP domain [aliases...]
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			// Skip the IP address, take domain names
			for i := 1; i < len(parts); i++ {
				domain := dfl.cleanDomain(parts[i])
				if domain != "" {
					domains = append(domains, domain)
				}
			}
		}
	}

	return domains, scanner.Err()
}

// parseAdblock parses AdBlock Plus filter format.
// Example: ||ads.example.com^
//          @@||whitelist.com^
func (dfl *DomainFileLoader) parseAdblock(content []byte) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines, comments, and whitelisted domains
		if line == "" || strings.HasPrefix(line, "!") || 
		   strings.HasPrefix(line, "[") || strings.HasPrefix(line, "@@") {
			continue
		}

		// Parse AdBlock rules
		domain := dfl.parseAdblockRule(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	return domains, scanner.Err()
}

// parseAdblockRule parses a single AdBlock rule.
func (dfl *DomainFileLoader) parseAdblockRule(rule string) string {
	// Remove common prefixes
	rule = strings.TrimPrefix(rule, "||")
	rule = strings.TrimPrefix(rule, "|")

	// Remove common suffixes
	rule = strings.TrimSuffix(rule, "^")
	rule = strings.TrimSuffix(rule, "$")

	// Skip element hiding rules
	if strings.Contains(rule, "##") || strings.Contains(rule, "#@#") {
		return ""
	}

	// Skip rules with options
	if strings.Contains(rule, "$") {
		parts := strings.Split(rule, "$")
		rule = parts[0]
	}

	// Extract domain
	if strings.Contains(rule, "/") {
		parts := strings.Split(rule, "/")
		rule = parts[0]
	}

	domain := dfl.cleanDomain(rule)
	return domain
}

// parseJSON parses JSON format domain lists.
// Supports multiple JSON structures.
func (dfl *DomainFileLoader) parseJSON(content []byte) ([]string, error) {
	var domains []string

	// Try array format: ["domain1", "domain2"]
	var arrayData []string
	if err := json.Unmarshal(content, &arrayData); err == nil {
		for _, domain := range arrayData {
			cleaned := dfl.cleanDomain(domain)
			if cleaned != "" {
				domains = append(domains, cleaned)
			}
		}
		return domains, nil
	}

	// Try object format: {"domains": ["domain1", "domain2"]}
	var objData map[string]interface{}
	if err := json.Unmarshal(content, &objData); err == nil {
		if domainsField, ok := objData["domains"]; ok {
			if domainsList, ok := domainsField.([]interface{}); ok {
				for _, d := range domainsList {
					if domain, ok := d.(string); ok {
						cleaned := dfl.cleanDomain(domain)
						if cleaned != "" {
							domains = append(domains, cleaned)
						}
					}
				}
			}
		}
		return domains, nil
	}

	return nil, fmt.Errorf("unsupported JSON format")
}

// isIPAddress checks if a string is a valid IPv4 address.
func (dfl *DomainFileLoader) isIPAddress(s string) bool {
	return dfl.isValidIPv4(s)
}

// cleanAndDeduplicate cleans and deduplicates domain list.
func (dfl *DomainFileLoader) cleanAndDeduplicate(domains []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(domains))

	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" && !seen[domain] {
			seen[domain] = true
			result = append(result, domain)
		}
	}

	return result
}

// GetLastDetectedFormat returns the last detected format.
// This is useful for updating configuration files with the detected format.
func (dfl *DomainFileLoader) GetLastDetectedFormat() string {
	return dfl.lastDetectedFormat
}

// ConvertFormat converts domain list from one format to another.
type FormatConverter struct {
	logger *slog.Logger
}

// NewFormatConverter creates a new format converter.
func NewFormatConverter(logger *slog.Logger) *FormatConverter {
	if logger == nil {
		logger = slog.Default()
	}
	return &FormatConverter{logger: logger}
}

// Convert converts domains from source format to target format.
func (fc *FormatConverter) Convert(domains []string, targetFormat string) ([]byte, error) {
	switch targetFormat {
	case "plain":
		return fc.toPlainText(domains), nil
	case "clash":
		return fc.toClashYAML(domains)
	case "surge":
		return fc.toSurge(domains), nil
	case "dnsmasq":
		return fc.toDnsmasq(domains), nil
	case "hosts":
		return fc.toHosts(domains), nil
	case "adblock":
		return fc.toAdblock(domains), nil
	case "json":
		return fc.toJSON(domains)
	default:
		return nil, fmt.Errorf("unsupported target format: %s", targetFormat)
	}
}

// toPlainText converts to plain text format.
func (fc *FormatConverter) toPlainText(domains []string) []byte {
	var buf strings.Builder
	buf.WriteString("# Domain list (plain text format)\n")
	buf.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, domain := range domains {
		buf.WriteString(strings.TrimSuffix(domain, ".") + "\n")
	}

	return []byte(buf.String())
}

// toClashYAML converts to Clash YAML format.
func (fc *FormatConverter) toClashYAML(domains []string) ([]byte, error) {
	var rules []string

	for _, domain := range domains {
		domain = strings.TrimSuffix(domain, ".")

		if strings.HasPrefix(domain, "*.") {
			// Wildcard domain -> DOMAIN-SUFFIX
			domain = strings.TrimPrefix(domain, "*.")
			rules = append(rules, fmt.Sprintf("  - DOMAIN-SUFFIX,%s", domain))
		} else if strings.Contains(domain, "*") {
			// Keyword match
			domain = strings.ReplaceAll(domain, "*", "")
			rules = append(rules, fmt.Sprintf("  - DOMAIN-KEYWORD,%s", domain))
		} else {
			// Exact match
			rules = append(rules, fmt.Sprintf("  - DOMAIN,%s", domain))
		}
	}

	var buf strings.Builder
	buf.WriteString("# Clash rule format\n")
	buf.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")
	buf.WriteString("payload:\n")
	buf.WriteString(strings.Join(rules, "\n"))
	buf.WriteString("\n")

	return []byte(buf.String()), nil
}

// toSurge converts to Surge format.
func (fc *FormatConverter) toSurge(domains []string) []byte {
	var buf strings.Builder
	buf.WriteString("# Surge rule format\n")
	buf.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, domain := range domains {
		domain = strings.TrimSuffix(domain, ".")

		if strings.HasPrefix(domain, "*.") {
			domain = strings.TrimPrefix(domain, "*.")
			buf.WriteString(fmt.Sprintf("DOMAIN-SUFFIX,%s\n", domain))
		} else if strings.Contains(domain, "*") {
			domain = strings.ReplaceAll(domain, "*", "")
			buf.WriteString(fmt.Sprintf("DOMAIN-KEYWORD,%s\n", domain))
		} else {
			buf.WriteString(fmt.Sprintf("DOMAIN,%s\n", domain))
		}
	}

	return []byte(buf.String())
}

// toDnsmasq converts to dnsmasq format.
func (fc *FormatConverter) toDnsmasq(domains []string) []byte {
	var buf strings.Builder
	buf.WriteString("# Dnsmasq configuration format\n")
	buf.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, domain := range domains {
		domain = strings.TrimSuffix(domain, ".")
		domain = strings.TrimPrefix(domain, "*.")
		buf.WriteString(fmt.Sprintf("server=/%s/\n", domain))
	}

	return []byte(buf.String())
}

// toHosts converts to hosts file format.
func (fc *FormatConverter) toHosts(domains []string) []byte {
	var buf strings.Builder
	buf.WriteString("# Hosts file format\n")
	buf.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, domain := range domains {
		domain = strings.TrimSuffix(domain, ".")
		domain = strings.TrimPrefix(domain, "*.")
		buf.WriteString(fmt.Sprintf("0.0.0.0 %s\n", domain))
	}

	return []byte(buf.String())
}

// toAdblock converts to AdBlock format.
func (fc *FormatConverter) toAdblock(domains []string) []byte {
	var buf strings.Builder
	buf.WriteString("! AdBlock Plus filter format\n")
	buf.WriteString("! Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, domain := range domains {
		domain = strings.TrimSuffix(domain, ".")
		domain = strings.TrimPrefix(domain, "*.")
		buf.WriteString(fmt.Sprintf("||%s^\n", domain))
	}

	return []byte(buf.String())
}

// toJSON converts to JSON format.
func (fc *FormatConverter) toJSON(domains []string) ([]byte, error) {
	// Clean domains for JSON
	cleaned := make([]string, len(domains))
	for i, domain := range domains {
		cleaned[i] = strings.TrimSuffix(domain, ".")
	}

	data := map[string]interface{}{
		"generated_at": time.Now().Format(time.RFC3339),
		"count":        len(cleaned),
		"domains":      cleaned,
	}

	return json.MarshalIndent(data, "", "  ")
}
