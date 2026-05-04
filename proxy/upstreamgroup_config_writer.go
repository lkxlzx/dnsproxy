package proxy

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// UpdateConfigFileStats updates domain_count and last_updated in the config file.
// This is useful for UI integration where the frontend needs to read these values.
func UpdateConfigFileStats(configPath string, stats map[string]DomainListStats) error {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	// Parse as yaml.Node to preserve structure and comments
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Find and update domains_lists section
	if err := updateDomainsListsStats(&root, stats); err != nil {
		return fmt.Errorf("update stats: %w", err)
	}

	// Write back to file
	output, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// DomainListStats contains statistics for a domain list.
type DomainListStats struct {
	DomainCount int
	LastUpdated time.Time
	Format      string // Detected format
}

// updateDomainsListsStats updates the domains_lists section with statistics.
func updateDomainsListsStats(node *yaml.Node, stats map[string]DomainListStats) error {
	// Navigate to the root document
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return fmt.Errorf("invalid yaml structure")
	}

	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("root is not a mapping")
	}

	// Find domains_lists key
	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valueNode := root.Content[i+1]

		if keyNode.Value == "domains_lists" && valueNode.Kind == yaml.SequenceNode {
			// Update each list in the sequence
			for _, listNode := range valueNode.Content {
				if listNode.Kind == yaml.MappingNode {
					updateListStats(listNode, stats)
				}
			}
			return nil
		}
	}

	return fmt.Errorf("domains_lists not found in config")
}

// updateListStats updates a single list's statistics.
func updateListStats(listNode *yaml.Node, stats map[string]DomainListStats) {
	var listName string

	// First pass: find the list name
	for i := 0; i < len(listNode.Content); i += 2 {
		keyNode := listNode.Content[i]
		valueNode := listNode.Content[i+1]

		if keyNode.Value == "name" {
			listName = valueNode.Value
			break
		}
	}

	if listName == "" {
		return
	}

	// Get stats for this list
	stat, exists := stats[listName]
	if !exists {
		return
	}

	// Second pass: update or add domain_count, last_updated, and format
	var hasDomainCount, hasLastUpdated, hasFormat bool

	for i := 0; i < len(listNode.Content); i += 2 {
		keyNode := listNode.Content[i]
		valueNode := listNode.Content[i+1]

		if keyNode.Value == "domain_count" {
			valueNode.Value = fmt.Sprintf("%d", stat.DomainCount)
			hasDomainCount = true
		} else if keyNode.Value == "last_updated" {
			valueNode.Value = stat.LastUpdated.Format(time.RFC3339)
			hasLastUpdated = true
		} else if keyNode.Value == "format" {
			// Update format if it was "auto" or empty
			if valueNode.Value == "" || valueNode.Value == "auto" {
				if stat.Format != "" {
					valueNode.Value = stat.Format
				}
			}
			hasFormat = true
		}
	}

	// Add missing fields
	if !hasDomainCount {
		listNode.Content = append(listNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "domain_count"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", stat.DomainCount)},
		)
	}

	if !hasLastUpdated {
		listNode.Content = append(listNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "last_updated"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: stat.LastUpdated.Format(time.RFC3339)},
		)
	}

	if !hasFormat && stat.Format != "" {
		listNode.Content = append(listNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "format"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: stat.Format},
		)
	}
}

// CollectDomainListStats collects statistics from parsed domain lists.
func CollectDomainListStats(spec *UpstreamGroupsSpec) (map[string]DomainListStats, error) {
	stats := make(map[string]DomainListStats)

	for _, listSpec := range spec.DomainLists {
		if !listSpec.Enabled || listSpec.File == "" {
			continue
		}

		// Read cache file to get domain count
		data, err := os.ReadFile(listSpec.File)
		if err != nil {
			continue // Skip if file doesn't exist yet
		}

		var cacheData struct {
			Domains []string `yaml:"domains"`
		}

		if err := yaml.Unmarshal(data, &cacheData); err != nil {
			continue
		}

		// Get file modification time
		info, err := os.Stat(listSpec.File)
		if err != nil {
			continue
		}

		stats[listSpec.Name] = DomainListStats{
			DomainCount: len(cacheData.Domains),
			LastUpdated: info.ModTime(),
			Format:      listSpec.Format, // Include detected format
		}
	}

	return stats, nil
}
