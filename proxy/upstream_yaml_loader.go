package proxy

import (
	"fmt"
	"os"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

// UpstreamGroupsYAML 定义YAML配置文件的结构（ADGH兼容格式）
type UpstreamGroupsYAML struct {
	// UpstreamDNS 默认上游DNS服务器列表
	UpstreamDNS []string `yaml:"upstream_dns"`

	// Groups 域名分组配置
	Groups []UpstreamGroupYAML `yaml:"upstream_dns_groups,omitempty"`
}

// UpstreamGroupYAML 单个上游分组的YAML配置
type UpstreamGroupYAML struct {
	// Name 分组名称
	Name string `yaml:"name"`

	// Domains 域名列表
	Domains []string `yaml:"domains"`

	// Upstreams 该分组使用的上游服务器
	Upstreams []string `yaml:"upstreams"`

	// SubdomainsOnly 是否仅匹配子域名（可选）
	SubdomainsOnly bool `yaml:"subdomains_only,omitempty"`
}

// LoadUpstreamConfigFromYAML 从YAML文件加载上游配置
// 支持ADGH兼容的YAML格式
//
// 示例YAML格式:
//
//	upstream_dns:
//	  - 8.8.8.8:53
//	  - 1.1.1.1:53
//
//	upstream_dns_groups:
//	  - name: china
//	    domains:
//	      - baidu.com
//	      - taobao.com
//	    upstreams:
//	      - 223.5.5.5:53
//	      - 119.29.29.29:53
//
//	  - name: company
//	    domains:
//	      - company.local
//	    upstreams:
//	      - 192.168.1.1:53
//	    subdomains_only: false
func LoadUpstreamConfigFromYAML(
	yamlFile string,
	opts *upstream.Options,
) (*UpstreamConfig, error) {
	// 读取YAML文件
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("read yaml file: %w", err)
	}

	// 解析YAML
	var config UpstreamGroupsYAML
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// 验证配置
	if len(config.UpstreamDNS) == 0 && len(config.Groups) == 0 {
		return nil, fmt.Errorf("no upstream dns servers configured")
	}

	// 转换为内部格式
	var lines []string

	// 处理每个分组
	for _, group := range config.Groups {
		if len(group.Domains) == 0 || len(group.Upstreams) == 0 {
			continue
		}

		// 为每个上游服务器生成配置行
		for _, upstreamAddr := range group.Upstreams {
			line := buildUpstreamLine(group.Domains, upstreamAddr, group.SubdomainsOnly)
			lines = append(lines, line)
		}
	}

	// 添加默认上游服务器
	lines = append(lines, config.UpstreamDNS...)

	// 解析配置
	return ParseUpstreamsConfig(lines, opts)
}

// LoadUpstreamConfigFromYAMLWithFile 从YAML文件和域名文件混合加载
// 支持在YAML中引用外部域名文件
//
// 示例YAML格式:
//
//	upstream_dns:
//	  - 8.8.8.8:53
//
//	upstream_dns_groups:
//	  - name: china
//	    domain_file: china_domains.txt  # 从文件加载域名
//	    upstreams:
//	      - 223.5.5.5:53
type UpstreamGroupYAMLWithFile struct {
	Name           string   `yaml:"name"`
	Domains        []string `yaml:"domains,omitempty"`
	DomainFile     string   `yaml:"domain_file,omitempty"`
	Upstreams      []string `yaml:"upstreams"`
	SubdomainsOnly bool     `yaml:"subdomains_only,omitempty"`
}

type UpstreamGroupsYAMLWithFile struct {
	UpstreamDNS []string                     `yaml:"upstream_dns"`
	Groups      []UpstreamGroupYAMLWithFile `yaml:"upstream_dns_groups,omitempty"`
}

// LoadUpstreamConfigFromYAMLWithFiles 从YAML文件加载配置，支持引用外部域名文件
func LoadUpstreamConfigFromYAMLWithFiles(
	yamlFile string,
	opts *upstream.Options,
) (*UpstreamConfig, error) {
	// 读取YAML文件
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("read yaml file: %w", err)
	}

	// 解析YAML
	var config UpstreamGroupsYAMLWithFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// 验证配置
	if len(config.UpstreamDNS) == 0 && len(config.Groups) == 0 {
		return nil, fmt.Errorf("no upstream dns servers configured")
	}

	// 转换为内部格式
	var lines []string

	// 处理每个分组
	for _, group := range config.Groups {
		var domains []string

		// 从文件加载域名
		if group.DomainFile != "" {
			fileDomains, err := loadDomainsFromFile(group.DomainFile)
			if err != nil {
				return nil, fmt.Errorf("load domains for group %s: %w", group.Name, err)
			}
			domains = append(domains, fileDomains...)
		}

		// 添加直接配置的域名
		domains = append(domains, group.Domains...)

		if len(domains) == 0 || len(group.Upstreams) == 0 {
			continue
		}

		// 为每个上游服务器生成配置行
		for _, upstreamAddr := range group.Upstreams {
			line := buildUpstreamLine(domains, upstreamAddr, group.SubdomainsOnly)
			lines = append(lines, line)
		}
	}

	// 添加默认上游服务器
	lines = append(lines, config.UpstreamDNS...)

	// 解析配置
	return ParseUpstreamsConfig(lines, opts)
}
