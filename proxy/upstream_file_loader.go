package proxy

import (
	"fmt"
	"strings"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// DomainGroupConfig 定义域名分组配置
type DomainGroupConfig struct {
	// GroupName 分组名称（用于日志）
	GroupName string
	
	// DomainFile 域名列表文件路径（每行一个域名）
	DomainFile string
	
	// DomainFileURL 域名列表远程URL（可选，用于自动更新）
	DomainFileURL string
	
	// DomainFileFormat 域名文件格式（可选，默认自动检测）
	DomainFileFormat DomainListFormat
	
	// UpdateInterval 自动更新间隔（0表示不自动更新）
	// 例如: "24h", "1h30m", "30m"
	UpdateInterval string
	
	// UpstreamGroupID 上游分组ID（引用UpstreamGroup）
	// 如果指定，则使用该分组的上游服务器（包括主备），忽略Upstreams和FallbackUpstreams字段
	UpstreamGroupID string
	
	// Upstreams 该分组使用的主要上游服务器列表（当UpstreamGroupID为空时使用）
	Upstreams []string
	
	// FallbackUpstreams 该分组使用的备用上游服务器列表（当UpstreamGroupID为空时使用）
	// 只有当所有主要上游都失败时才使用备用上游
	FallbackUpstreams []string
	
	// DefaultUpstreamGroupID 默认上游分组ID（最终兜底）
	// 当主要上游和备用上游都失败时，使用此分组ID指定的上游
	// 如果为空，则使用全局默认上游
	DefaultUpstreamGroupID string
	
	// SubdomainsOnly 是否仅匹配子域名（不包括域名本身）
	SubdomainsOnly bool
	
	// Enabled 是否启用该规则组（默认true）
	Enabled bool
	
	// domains 缓存的域名列表（内部使用，运行时加载）
	domains []string
	
	// domainCount 有效域名数量（内部使用）
	domainCount int
	
	// lastUpdate 最后更新时间（内部使用）
	lastUpdate string
}

// LoadUpstreamConfigFromFiles 从文件加载域名分组配置
// 参数:
//   - groups: 域名分组配置列表
//   - defaultUpstreams: 默认上游服务器列表
//   - opts: 上游服务器选项
//
// 返回:
//   - *UpstreamConfig: 解析后的上游配置
//   - error: 错误信息
func LoadUpstreamConfigFromFiles(
	groups []DomainGroupConfig,
	defaultUpstreams []string,
	opts *upstream.Options,
) (*UpstreamConfig, error) {
	var lines []string

	// 处理每个域名分组
	for _, group := range groups {
		// 使用转换器加载域名
		converter := NewDomainListConverter(group.DomainFile, group.DomainFileFormat)
		upstreamLines, err := converter.ConvertToUpstreamLines(group.Upstreams, group.SubdomainsOnly)
		if err != nil {
			return nil, fmt.Errorf("convert group %s: %w", group.GroupName, err)
		}

		lines = append(lines, upstreamLines...)
	}

	// 添加默认上游服务器
	lines = append(lines, defaultUpstreams...)

	// 解析配置
	return ParseUpstreamsConfig(lines, opts)
}

// buildUpstreamLine 构建上游配置行
func buildUpstreamLine(domains []string, upstreamAddr string, subdomainsOnly bool) string {
	var domainSpecs []string

	for _, domain := range domains {
		if subdomainsOnly {
			domainSpecs = append(domainSpecs, "*."+domain)
		} else {
			domainSpecs = append(domainSpecs, domain)
		}
	}

	return fmt.Sprintf("[/%s/]%s", strings.Join(domainSpecs, "/"), upstreamAddr)
}

// LoadUpstreamConfigFromFileSimple 简化版本：从单个文件加载域名并指定上游
func LoadUpstreamConfigFromFileSimple(
	domainFile string,
	upstreamAddrs []string,
	defaultUpstreams []string,
	opts *upstream.Options,
) (*UpstreamConfig, error) {
	group := DomainGroupConfig{
		GroupName:      "custom",
		DomainFile:     domainFile,
		Upstreams:      upstreamAddrs,
		SubdomainsOnly: false,
	}

	return LoadUpstreamConfigFromFiles([]DomainGroupConfig{group}, defaultUpstreams, opts)
}
