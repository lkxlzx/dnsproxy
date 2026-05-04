package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIHandler provides HTTP handlers for domain list management
type APIHandler struct {
	hotReload *HotReloadManager
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(hotReload *HotReloadManager) *APIHandler {
	return &APIHandler{
		hotReload: hotReload,
	}
}

// HandleAddDomainList handles POST /api/domain-lists
func (h *APIHandler) HandleAddDomainList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var list DomainListSpec
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if list.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if list.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}
	if list.Group == "" {
		http.Error(w, "group is required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if list.File == "" {
		list.File = fmt.Sprintf("./cache/%s.yaml", list.Name)
	}
	if list.RefreshInterval == "" {
		list.RefreshInterval = "24h"
	}
	if !list.Enabled {
		list.Enabled = true
	}

	// Add domain list
	if err := h.hotReload.AddDomainList(list); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add domain list: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Domain list %q added successfully", list.Name),
		"list":    list,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleRemoveDomainList handles DELETE /api/domain-lists/{name}
func (h *APIHandler) HandleRemoveDomainList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}

	if err := h.hotReload.RemoveDomainList(name); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove domain list: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Domain list %q removed successfully", name),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleUpdateDomainList handles PUT /api/domain-lists/{name}
func (h *APIHandler) HandleUpdateDomainList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if err := h.hotReload.UpdateDomainList(name, updates); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update domain list: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Domain list %q updated successfully", name),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleReloadConfig handles POST /api/reload
func (h *APIHandler) HandleReloadConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.hotReload.Reload(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Configuration reloaded successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
