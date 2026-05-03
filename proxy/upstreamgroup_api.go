package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIConfig contains configuration for the HTTP API.
type APIConfig struct {
	// Enabled indicates whether the API is enabled.
	Enabled bool

	// ListenAddr is the address to listen on (e.g., "127.0.0.1:8080").
	ListenAddr string

	// ReadTimeout is the HTTP read timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the HTTP write timeout.
	WriteTimeout time.Duration

	// AuthToken is the optional bearer token for authentication.
	AuthToken string
}

// DefaultAPIConfig returns the default API configuration.
func DefaultAPIConfig() *APIConfig {
	return &APIConfig{
		Enabled:      false,
		ListenAddr:   "127.0.0.1:8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// APIServer provides HTTP API for managing upstream groups.
type APIServer struct {
	config         *APIConfig
	logger         *slog.Logger
	manager        *HotReloadManager
	server         *http.Server
	mu             sync.RWMutex
}

// NewAPIServer creates a new API server.
func NewAPIServer(
	config *APIConfig,
	manager *HotReloadManager,
	logger *slog.Logger,
) *APIServer {
	if config == nil {
		config = DefaultAPIConfig()
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &APIServer{
		config:  config,
		logger:  logger,
		manager: manager,
	}
}

// Start starts the API server.
func (api *APIServer) Start() error {
	if !api.config.Enabled {
		api.logger.Info("API server is disabled")
		return nil
	}

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/v1/groups", api.authMiddleware(api.handleGroups))
	mux.HandleFunc("/api/v1/groups/", api.authMiddleware(api.handleGroup))
	mux.HandleFunc("/api/v1/stats", api.authMiddleware(api.handleStats))
	mux.HandleFunc("/api/v1/stats/", api.authMiddleware(api.handleGroupStats))
	mux.HandleFunc("/api/v1/health", api.authMiddleware(api.handleHealth))
	mux.HandleFunc("/api/v1/health/", api.authMiddleware(api.handleUpstreamHealth))
	mux.HandleFunc("/api/v1/reload", api.authMiddleware(api.handleReload))
	mux.HandleFunc("/api/v1/status", api.handleStatus)

	api.server = &http.Server{
		Addr:         api.config.ListenAddr,
		Handler:      mux,
		ReadTimeout:  api.config.ReadTimeout,
		WriteTimeout: api.config.WriteTimeout,
	}

	go func() {
		api.logger.Info("API server starting", "addr", api.config.ListenAddr)
		if err := api.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			api.logger.Error("API server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the API server.
func (api *APIServer) Stop() error {
	if api.server == nil {
		return nil
	}

	api.logger.Info("stopping API server")
	return api.server.Close()
}

// authMiddleware provides authentication middleware.
func (api *APIServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if api.config.AuthToken != "" {
			authHeader := r.Header.Get("Authorization")
			expectedAuth := "Bearer " + api.config.AuthToken

			if authHeader != expectedAuth {
				api.writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}

		next(w, r)
	}
}

// handleGroups handles GET /api/v1/groups
func (api *APIServer) handleGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	conf := api.manager.GetCurrentConfig()
	if conf == nil {
		api.writeError(w, http.StatusInternalServerError, "no configuration loaded")
		return
	}

	groups := make([]GroupInfo, 0, len(conf.Groups))
	for name, group := range conf.Groups {
		upstreams := make([]string, len(group.Upstreams))
		for i, u := range group.Upstreams {
			upstreams[i] = u.Address()
		}

		groups = append(groups, GroupInfo{
			Name:       name,
			Upstreams:  upstreams,
			Mode:       string(group.Mode),
			Timeout:    group.Timeout.String(),
			MaxRetries: group.MaxRetries,
			Enabled:    group.Enabled,
			Priority:   group.Priority,
		})
	}

	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"groups":        groups,
		"default_group": conf.DefaultGroup,
		"total":         len(groups),
	})
}

// handleGroup handles GET /api/v1/groups/{name}
func (api *APIServer) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	groupName := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	if groupName == "" {
		api.writeError(w, http.StatusBadRequest, "group name required")
		return
	}

	conf := api.manager.GetCurrentConfig()
	if conf == nil {
		api.writeError(w, http.StatusInternalServerError, "no configuration loaded")
		return
	}

	group, exists := conf.Groups[groupName]
	if !exists {
		api.writeError(w, http.StatusNotFound, "group not found")
		return
	}

	upstreams := make([]string, len(group.Upstreams))
	for i, u := range group.Upstreams {
		upstreams[i] = u.Address()
	}

	api.writeJSON(w, http.StatusOK, GroupInfo{
		Name:       group.Name,
		Upstreams:  upstreams,
		Mode:       string(group.Mode),
		Timeout:    group.Timeout.String(),
		MaxRetries: group.MaxRetries,
		Enabled:    group.Enabled,
		Priority:   group.Priority,
	})
}

// handleStats handles GET /api/v1/stats
func (api *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	collector := api.manager.GetStatsCollector()
	if collector == nil {
		api.writeError(w, http.StatusInternalServerError, "stats collector not available")
		return
	}

	allStats := collector.GetAllStats()
	api.writeJSON(w, http.StatusOK, allStats)
}

// handleGroupStats handles GET /api/v1/stats/{group}
func (api *APIServer) handleGroupStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	groupName := strings.TrimPrefix(r.URL.Path, "/api/v1/stats/")
	if groupName == "" {
		api.writeError(w, http.StatusBadRequest, "group name required")
		return
	}

	collector := api.manager.GetStatsCollector()
	if collector == nil {
		api.writeError(w, http.StatusInternalServerError, "stats collector not available")
		return
	}

	stats, err := collector.GetGroupStats(groupName)
	if err != nil {
		api.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, stats.GetSnapshot())
}

// handleHealth handles GET /api/v1/health
func (api *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	checker := api.manager.GetHealthChecker()
	if checker == nil {
		api.writeError(w, http.StatusInternalServerError, "health checker not available")
		return
	}

	allStats := checker.GetAllStats()
	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"upstreams": allStats,
		"total":     len(allStats),
	})
}

// handleUpstreamHealth handles GET /api/v1/health/{address}
func (api *APIServer) handleUpstreamHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	address := strings.TrimPrefix(r.URL.Path, "/api/v1/health/")
	if address == "" {
		api.writeError(w, http.StatusBadRequest, "upstream address required")
		return
	}

	checker := api.manager.GetHealthChecker()
	if checker == nil {
		api.writeError(w, http.StatusInternalServerError, "health checker not available")
		return
	}

	stats, err := checker.GetStats(address)
	if err != nil {
		api.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, stats)
}

// handleReload handles POST /api/v1/reload
func (api *APIServer) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := api.manager.Reload(); err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "configuration reloaded successfully",
	})
}

// handleStatus handles GET /api/v1/status
func (api *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	conf := api.manager.GetCurrentConfig()
	status := map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
	}

	if conf != nil {
		status["groups_count"] = len(conf.Groups)
		status["default_group"] = conf.DefaultGroup
	}

	api.writeJSON(w, http.StatusOK, status)
}

// writeJSON writes a JSON response.
func (api *APIServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logger.Error("failed to encode JSON", "error", err)
	}
}

// writeError writes an error response.
func (api *APIServer) writeError(w http.ResponseWriter, status int, message string) {
	api.writeJSON(w, status, map[string]interface{}{
		"error": message,
	})
}

// GroupInfo contains information about a group for API responses.
type GroupInfo struct {
	Name       string   `json:"name"`
	Upstreams  []string `json:"upstreams"`
	Mode       string   `json:"mode"`
	Timeout    string   `json:"timeout"`
	MaxRetries int      `json:"max_retries"`
	Enabled    bool     `json:"enabled"`
	Priority   int      `json:"priority"`
}
