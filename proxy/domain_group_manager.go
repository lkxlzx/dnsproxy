package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// DomainGroupManager 管理域名分组和动态路由
type DomainGroupManager struct {
	mu              sync.RWMutex
	groups          []DomainGroupConfig
	opts            *upstream.Options
	upstreamGroupMgr *UpstreamGroupManager
	listUpdater     *DomainListUpdater
}

// NewDomainGroupManager 创建域名分组管理器
func NewDomainGroupManager(
	groups []DomainGroupConfig,
	upstreamGroups []UpstreamGroup,
	opts *upstream.Options,
	logger *slog.Logger,
) (*DomainGroupManager, error) {
	// 创建上游分组管理器
	upstreamGroupMgr := NewUpstreamGroupManager()
	for _, group := range upstreamGroups {
		if err := upstreamGroupMgr.AddGroup(&group); err != nil {
			return nil, fmt.Errorf("add upstream group: %w", err)
		}
	}

	// 创建域名列表更新器
	listUpdater := NewDomainListUpdater(logger)

	mgr := &DomainGroupManager{
		groups:          groups,
		opts:            opts,
		upstreamGroupMgr: upstreamGroupMgr,
		listUpdater:     listUpdater,
	}

	// 加载所有域名列表并设置自动更新
	if err := mgr.loadAllDomains(); err != nil {
		return nil, fmt.Errorf("load domains: %w", err)
	}

	return mgr, nil
}

// loadAllDomains 加载所有分组的域名列表
func (m *DomainGroupManager) loadAllDomains() error {
	for i := range m.groups {
		group := &m.groups[i]
		
		// 默认启用
		if !group.Enabled {
			group.Enabled = true
		}

		// 如果有URL，设置自动更新
		if group.DomainFileURL != "" {
			updateInterval, err := parseDuration(group.UpdateInterval)
			if err != nil {
				return fmt.Errorf("parse update interval for group %s: %w", group.GroupName, err)
			}

			source := &DomainListSource{
				FilePath:       group.DomainFile,
				URL:            group.DomainFileURL,
				Format:         formatToString(group.DomainFileFormat),
				UpdateInterval: updateInterval,
			}

			if err := m.listUpdater.AddSource(group.GroupName, source); err != nil {
				return fmt.Errorf("add update source for group %s: %w", group.GroupName, err)
			}

			// 如果文件不存在，立即下载
			if !fileExists(group.DomainFile) {
				if err := m.listUpdater.UpdateNow(group.GroupName); err != nil {
					return fmt.Errorf("initial download for group %s: %w", group.GroupName, err)
				}
			}
		}

		// 加载域名列表
		converter := NewDomainListConverter(group.DomainFile, group.DomainFileFormat)
		domains, err := converter.ConvertToDomains()
		if err != nil {
			return fmt.Errorf("load group %s: %w", group.GroupName, err)
		}

		group.domains = domains
		group.domainCount = len(domains)
		group.lastUpdate = time.Now().Format(time.RFC3339)
	}

	return nil
}

// parseDuration parses a duration string, returns 0 if empty.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// MatchDomain 匹配域名并返回对应的上游服务器
// 返回 nil 表示没有匹配的规则，应使用默认上游
func (m *DomainGroupManager) MatchDomain(domain string) ([]upstream.Upstream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	for _, group := range m.groups {
		// 跳过禁用的规则组
		if !group.Enabled {
			continue
		}

		if m.matchDomainInGroup(domain, &group) {
			// 解析主要上游地址（从分组ID或直接地址）
			primaryAddrs, fallbackAddrs, err := m.upstreamGroupMgr.ResolveUpstreams(group.UpstreamGroupID, group.Upstreams)
			if err != nil {
				return nil, fmt.Errorf("resolve upstreams for group %s: %w", group.GroupName, err)
			}

			// 如果没有使用分组ID，使用直接配置的fallback
			if group.UpstreamGroupID == "" && len(group.FallbackUpstreams) > 0 {
				fallbackAddrs = group.FallbackUpstreams
			}

			// 创建主要上游服务器
			upstreams := make([]upstream.Upstream, 0, len(primaryAddrs))
			for _, addr := range primaryAddrs {
				u, err := upstream.AddressToUpstream(addr, m.opts)
				if err != nil {
					return nil, fmt.Errorf("create upstream %s: %w", addr, err)
				}
				upstreams = append(upstreams, u)
			}

			// 如果有fallback，创建fallback上游并添加到列表末尾
			// 注意：这里我们将fallback添加到同一个列表中
			// 实际的fallback逻辑需要在查询时处理（类似全局Fallbacks）
			if len(fallbackAddrs) > 0 {
				for _, addr := range fallbackAddrs {
					u, err := upstream.AddressToUpstream(addr, m.opts)
					if err != nil {
						return nil, fmt.Errorf("create fallback upstream %s: %w", addr, err)
					}
					upstreams = append(upstreams, u)
				}
			}

			return upstreams, nil
		}
	}

	return nil, nil
}

// MatchDomainWithFallback 匹配域名并返回主要和备用上游服务器
// 返回 (primaryUpstreams, fallbackUpstreams, error)
// 如果没有匹配的规则，返回 (nil, nil, nil)
// 注意：不再返回defaultUpstreams，因为全局upstream应该在proxy层处理
func (m *DomainGroupManager) MatchDomainWithFallback(domain string) ([]upstream.Upstream, []upstream.Upstream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	for _, group := range m.groups {
		// 跳过禁用的规则组
		if !group.Enabled {
			continue
		}

		if m.matchDomainInGroup(domain, &group) {
			// 解析主要和备用上游地址
			primaryAddrs, fallbackAddrs, err := m.upstreamGroupMgr.ResolveUpstreams(group.UpstreamGroupID, group.Upstreams)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve upstreams for group %s: %w", group.GroupName, err)
			}

			// 如果没有使用分组ID，使用直接配置的fallback
			if group.UpstreamGroupID == "" && len(group.FallbackUpstreams) > 0 {
				fallbackAddrs = group.FallbackUpstreams
			}

			// 创建主要上游服务器
			primaryUpstreams := make([]upstream.Upstream, 0, len(primaryAddrs))
			for _, addr := range primaryAddrs {
				u, err := upstream.AddressToUpstream(addr, m.opts)
				if err != nil {
					return nil, nil, fmt.Errorf("create upstream %s: %w", addr, err)
				}
				primaryUpstreams = append(primaryUpstreams, u)
			}

			// 创建备用上游服务器
			var fallbackUpstreams []upstream.Upstream
			if len(fallbackAddrs) > 0 {
				fallbackUpstreams = make([]upstream.Upstream, 0, len(fallbackAddrs))
				for _, addr := range fallbackAddrs {
					u, err := upstream.AddressToUpstream(addr, m.opts)
					if err != nil {
						return nil, nil, fmt.Errorf("create fallback upstream %s: %w", addr, err)
					}
					fallbackUpstreams = append(fallbackUpstreams, u)
				}
			}

			return primaryUpstreams, fallbackUpstreams, nil
		}
	}

	return nil, nil, nil
}

// GetDefaultUpstreams returns the upstreams for unmatched domains based on DefaultUpstreamGroupID.
// Returns (primaryUpstreams, fallbackUpstreams, error).
// If DefaultUpstreamGroupID is not set or group not found, returns (nil, nil, nil).
func (m *DomainGroupManager) GetDefaultUpstreams(defaultUpstreamGroupID string) ([]upstream.Upstream, []upstream.Upstream, error) {
	if defaultUpstreamGroupID == "" {
		return nil, nil, nil
	}

	primaryAddrs, fallbackAddrs, err := m.upstreamGroupMgr.GetAllUpstreams(defaultUpstreamGroupID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve default upstream group %s: %w", defaultUpstreamGroupID, err)
	}

	// 创建主要上游服务器
	var primaryUpstreams []upstream.Upstream
	if len(primaryAddrs) > 0 {
		primaryUpstreams = make([]upstream.Upstream, 0, len(primaryAddrs))
		for _, addr := range primaryAddrs {
			u, err := upstream.AddressToUpstream(addr, m.opts)
			if err != nil {
				return nil, nil, fmt.Errorf("create default upstream %s: %w", addr, err)
			}
			primaryUpstreams = append(primaryUpstreams, u)
		}
	}

	// 创建备用上游服务器
	var fallbackUpstreams []upstream.Upstream
	if len(fallbackAddrs) > 0 {
		fallbackUpstreams = make([]upstream.Upstream, 0, len(fallbackAddrs))
		for _, addr := range fallbackAddrs {
			u, err := upstream.AddressToUpstream(addr, m.opts)
			if err != nil {
				return nil, nil, fmt.Errorf("create default fallback upstream %s: %w", addr, err)
			}
			fallbackUpstreams = append(fallbackUpstreams, u)
		}
	}

	return primaryUpstreams, fallbackUpstreams, nil
}

// matchDomainInGroup 检查域名是否匹配分组中的任一域名
func (m *DomainGroupManager) matchDomainInGroup(domain string, group *DomainGroupConfig) bool {
	for _, pattern := range group.domains {
		pattern = strings.ToLower(strings.TrimSuffix(pattern, "."))

		// 检查是否是关键字模式 (keyword:ad)
		if strings.HasPrefix(pattern, "keyword:") {
			keyword := pattern[8:] // 移除 "keyword:"
			if strings.Contains(domain, keyword) {
				return true
			}
			continue
		}

		// 检查是否是通配符模式 (*.example.com)
		if strings.HasPrefix(pattern, "*.") {
			// 通配符模式：仅匹配子域名
			baseDomain := pattern[2:] // 移除 "*."
			if strings.HasSuffix(domain, "."+baseDomain) {
				return true
			}
		} else if group.SubdomainsOnly {
			// SubdomainsOnly 模式：仅匹配子域名
			if strings.HasSuffix(domain, "."+pattern) {
				return true
			}
		} else {
			// 普通模式：匹配域名本身及其子域名
			if domain == pattern || strings.HasSuffix(domain, "."+pattern) {
				return true
			}
		}
	}

	return false
}

// EnableGroup 启用指定的规则组
func (m *DomainGroupManager) EnableGroup(groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.groups {
		if m.groups[i].GroupName == groupName {
			m.groups[i].Enabled = true
			return nil
		}
	}

	return fmt.Errorf("group not found: %s", groupName)
}

// DisableGroup 禁用指定的规则组
func (m *DomainGroupManager) DisableGroup(groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.groups {
		if m.groups[i].GroupName == groupName {
			m.groups[i].Enabled = false
			return nil
		}
	}

	return fmt.Errorf("group not found: %s", groupName)
}

// ReloadGroup 重新加载指定规则组的域名列表
func (m *DomainGroupManager) ReloadGroup(groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.groups {
		if m.groups[i].GroupName == groupName {
			group := &m.groups[i]
			converter := NewDomainListConverter(group.DomainFile, group.DomainFileFormat)
			domains, err := converter.ConvertToDomains()
			if err != nil {
				return fmt.Errorf("reload group %s: %w", groupName, err)
			}
			group.domains = domains
			return nil
		}
	}

	return fmt.Errorf("group not found: %s", groupName)
}

// GetGroups 获取所有规则组的状态
func (m *DomainGroupManager) GetGroups() []DomainGroupStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make([]DomainGroupStatus, len(m.groups))
	for i, group := range m.groups {
		// 获取上游信息
		upstreamInfo := ""
		if group.UpstreamGroupID != "" {
			upstreamInfo = "group:" + group.UpstreamGroupID
		} else {
			upstreamInfo = fmt.Sprintf("%d upstreams", len(group.Upstreams))
		}

		status[i] = DomainGroupStatus{
			GroupName:       group.GroupName,
			DomainFile:      group.DomainFile,
			DomainFileURL:   group.DomainFileURL,
			Enabled:         group.Enabled,
			DomainCount:     group.domainCount,
			UpstreamInfo:    upstreamInfo,
			SubdomainsOnly:  group.SubdomainsOnly,
			UpdateInterval:  group.UpdateInterval,
			LastUpdate:      group.lastUpdate,
		}
	}

	return status
}

// DomainGroupStatus 规则组状态信息
type DomainGroupStatus struct {
	GroupName      string `json:"group_name"`
	DomainFile     string `json:"domain_file"`
	DomainFileURL  string `json:"domain_file_url,omitempty"`
	Enabled        bool   `json:"enabled"`
	DomainCount    int    `json:"domain_count"`
	UpstreamInfo   string `json:"upstream_info"`
	SubdomainsOnly bool   `json:"subdomains_only"`
	UpdateInterval string `json:"update_interval,omitempty"`
	LastUpdate     string `json:"last_update,omitempty"`
}

// UpdateGroup 手动触发域名列表更新
func (m *DomainGroupManager) UpdateGroup(groupName string) error {
	// 触发下载更新
	if err := m.listUpdater.UpdateNow(groupName); err != nil {
		return fmt.Errorf("update from URL: %w", err)
	}

	// 重新加载域名列表
	return m.ReloadGroup(groupName)
}

// GetUpdateStatus 获取所有域名列表的更新状态
func (m *DomainGroupManager) GetUpdateStatus() map[string]DomainListSourceStatus {
	return m.listUpdater.GetStatus()
}

// Stop 停止域名分组管理器（停止自动更新）
func (m *DomainGroupManager) Stop() {
	m.listUpdater.Stop()
}


// formatToString converts DomainListFormat to string
func formatToString(format DomainListFormat) string {
	switch format {
	case FormatPlain:
		return "plain"
	case FormatDnsmasq:
		return "dnsmasq"
	case FormatGFWList:
		return "gfwlist"
	case FormatClash:
		return "clash"
	default:
		return ""
	}
}
