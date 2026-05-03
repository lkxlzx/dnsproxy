package proxy

import (
	"strings"
	"sync"
)

// DomainTrie is a trie (prefix tree) optimized for domain name matching.
// It stores domains in reverse order (com.example.www) for efficient suffix matching.
// This is particularly useful for wildcard domain matching (*.example.com).
//
// Performance characteristics:
//   - Insert: O(m) where m is the length of the domain
//   - Search: O(m) where m is the length of the domain
//   - Memory: O(n*m) where n is the number of domains and m is average domain length
//
// Thread-safe: All operations are protected by RWMutex.
type DomainTrie struct {
	root *trieNode
	mu   sync.RWMutex
}

// trieNode represents a node in the domain trie.
type trieNode struct {
	children map[string]*trieNode // Map of label -> child node
	groupName string              // Group name if this node represents a complete domain
	isEnd     bool                // True if this node marks the end of a domain
	isWildcard bool               // True if this node represents a wildcard (*)
}

// NewDomainTrie creates a new domain trie for efficient domain matching.
func NewDomainTrie() *DomainTrie {
	return &DomainTrie{
		root: &trieNode{
			children: make(map[string]*trieNode),
		},
	}
}

// Insert adds a domain and its associated group name to the trie.
// Domains are stored in reverse order for efficient suffix matching.
//
// Examples:
//   - "example.com" -> stored as ["com", "example"]
//   - "*.example.com" -> stored as ["com", "example", "*"]
//   - "www.example.com" -> stored as ["com", "example", "www"]
func (dt *DomainTrie) Insert(domain, groupName string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Normalize: remove trailing dot
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return
	}

	// Split domain into labels and reverse them
	// "www.example.com" -> ["com", "example", "www"]
	labels := strings.Split(domain, ".")
	reverseLabels(labels)

	// Insert into trie
	node := dt.root
	for _, label := range labels {
		if node.children == nil {
			node.children = make(map[string]*trieNode)
		}

		if _, exists := node.children[label]; !exists {
			node.children[label] = &trieNode{
				children: make(map[string]*trieNode),
			}
		}

		node = node.children[label]
		
		// Mark wildcard nodes
		if label == "*" {
			node.isWildcard = true
		}
	}

	// Mark end of domain
	node.isEnd = true
	node.groupName = groupName
}

// Search finds the best matching group for a given domain.
// It performs the following matching in order:
//   1. Exact match: "www.example.com" matches "www.example.com"
//   2. Wildcard match: "www.example.com" matches "*.example.com"
//   3. Partial wildcard: "www.example.com" matches "*.com"
//
// Returns the group name and true if found, empty string and false otherwise.
func (dt *DomainTrie) Search(domain string) (string, bool) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	// Normalize: remove trailing dot
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "", false
	}

	// Split domain into labels and reverse them
	labels := strings.Split(domain, ".")
	reverseLabels(labels)

	// Try to find the best match
	return dt.searchNode(dt.root, labels, 0)
}

// searchNode recursively searches for the best matching domain.
// It tries exact matches first, then wildcard matches.
func (dt *DomainTrie) searchNode(node *trieNode, labels []string, depth int) (string, bool) {
	// If we've consumed all labels
	if depth >= len(labels) {
		if node.isEnd {
			return node.groupName, true
		}
		return "", false
	}

	currentLabel := labels[depth]

	// Try exact match first
	if child, exists := node.children[currentLabel]; exists {
		// Continue searching deeper
		if groupName, found := dt.searchNode(child, labels, depth+1); found {
			return groupName, true
		}
		
		// If this node is an end node, it's a valid match
		if child.isEnd {
			return child.groupName, true
		}
	}

	// Try wildcard match
	if wildcardChild, exists := node.children["*"]; exists {
		if wildcardChild.isEnd {
			// Wildcard matches the rest of the domain
			return wildcardChild.groupName, true
		}
	}

	return "", false
}

// Delete removes a domain from the trie.
func (dt *DomainTrie) Delete(domain string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Normalize: remove trailing dot
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return
	}

	// Split domain into labels and reverse them
	labels := strings.Split(domain, ".")
	reverseLabels(labels)

	// Delete from trie
	dt.deleteNode(dt.root, labels, 0)
}

// deleteNode recursively deletes a domain from the trie.
func (dt *DomainTrie) deleteNode(node *trieNode, labels []string, depth int) bool {
	if depth >= len(labels) {
		if node.isEnd {
			node.isEnd = false
			node.groupName = ""
		}
		return len(node.children) == 0 && !node.isEnd
	}

	currentLabel := labels[depth]
	child, exists := node.children[currentLabel]
	if !exists {
		return false
	}

	shouldDeleteChild := dt.deleteNode(child, labels, depth+1)
	if shouldDeleteChild {
		delete(node.children, currentLabel)
	}

	return len(node.children) == 0 && !node.isEnd
}

// Clear removes all domains from the trie.
func (dt *DomainTrie) Clear() {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.root = &trieNode{
		children: make(map[string]*trieNode),
	}
}

// Size returns the approximate number of domains in the trie.
// This is an O(n) operation that traverses the entire trie.
func (dt *DomainTrie) Size() int {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	return dt.countNodes(dt.root)
}

// countNodes recursively counts the number of end nodes (complete domains).
func (dt *DomainTrie) countNodes(node *trieNode) int {
	count := 0
	if node.isEnd {
		count = 1
	}

	for _, child := range node.children {
		count += dt.countNodes(child)
	}

	return count
}

// reverseLabels reverses a slice of strings in place.
// Example: ["www", "example", "com"] -> ["com", "example", "www"]
func reverseLabels(labels []string) {
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
}
