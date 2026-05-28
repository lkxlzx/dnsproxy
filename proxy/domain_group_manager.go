package proxy

import (
	"fmt"
	"strings"
	"sync"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// DomainGroupManager 管理域名分组和动态路由
type DomainGroupManager struct {
	mu     sync.RWMutex
	groups []DomainGroupConfig
	opts   *upstream.Options
}

// NewDomainGroupManager 创建域名分组管理器
func NewDomainGroupManager(groups []DomainGroupConfig, opts *upstream.Options) (*DomainGroupManager, error) {
	mgr := &DomainGroupManager{
		groups: groups,
		opts:   opts,
	}

	// 加载所有域名列表
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

		converter := NewDomainListConverter(group.DomainFile, group.DomainFileFormat)
		domains, err := converter.ConvertToDomains()
		if err != nil {
			return fmt.Errorf("load group %s: %w", group.GroupName, err)
		}

		group.domains = domains
	}

	return nil
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
			// 创建上游服务器
			upstreams := make([]upstream.Upstream, 0, len(group.Upstreams))
			for _, addr := range group.Upstreams {
				u, err := upstream.AddressToUpstream(addr, m.opts)
				if err != nil {
					return nil, fmt.Errorf("create upstream %s: %w", addr, err)
				}
				upstreams = append(upstreams, u)
			}
			return upstreams, nil
		}
	}

	return nil, nil
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
		status[i] = DomainGroupStatus{
			GroupName:      group.GroupName,
			DomainFile:     group.DomainFile,
			Enabled:        group.Enabled,
			DomainCount:    len(group.domains),
			UpstreamCount:  len(group.Upstreams),
			SubdomainsOnly: group.SubdomainsOnly,
		}
	}

	return status
}

// DomainGroupStatus 规则组状态信息
type DomainGroupStatus struct {
	GroupName      string
	DomainFile     string
	Enabled        bool
	DomainCount    int
	UpstreamCount  int
	SubdomainsOnly bool
}
