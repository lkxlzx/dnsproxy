package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DomainListConverter 域名列表格式转换器
// 将各种格式的域名列表统一转换为dnsproxy原生格式
type DomainListConverter struct {
	// SourceFormat 源文件格式
	SourceFormat DomainListFormat
	// SourceFile 源文件路径
	SourceFile string
}

// DomainListFormat 域名列表格式类型
type DomainListFormat int

const (
	// FormatAuto 自动检测格式
	FormatAuto DomainListFormat = iota
	// FormatPlain 纯文本格式（每行一个域名）
	FormatPlain
	// FormatDnsmasq dnsmasq格式（server=/domain/dns）
	FormatDnsmasq
	// FormatGFWList GFWList格式（Base64编码）
	FormatGFWList
	// FormatClash Clash规则格式（YAML）
	FormatClash
)

// NewDomainListConverter 创建域名列表转换器
func NewDomainListConverter(sourceFile string, format DomainListFormat) *DomainListConverter {
	return &DomainListConverter{
		SourceFile:   sourceFile,
		SourceFormat: format,
	}
}

// ConvertToDomains 转换为域名列表
// 这是统一的转换接口，所有格式最终都转换为简单的域名列表
func (c *DomainListConverter) ConvertToDomains() ([]string, error) {
	// 自动检测格式
	if c.SourceFormat == FormatAuto {
		format, err := c.detectFormat()
		if err != nil {
			return nil, fmt.Errorf("detect format: %w", err)
		}
		c.SourceFormat = format
	}

	// 根据格式转换
	switch c.SourceFormat {
	case FormatPlain:
		return c.convertPlain()
	case FormatDnsmasq:
		return c.convertDnsmasq()
	case FormatGFWList:
		return c.convertGFWList()
	case FormatClash:
		return c.convertClash()
	default:
		return nil, fmt.Errorf("unsupported format: %d", c.SourceFormat)
	}
}

// ConvertToUpstreamLines 转换为dnsproxy上游配置行
// 这是最终的转换目标：[/domain1/domain2/]upstream
func (c *DomainListConverter) ConvertToUpstreamLines(upstreams []string, subdomainsOnly bool) ([]string, error) {
	domains, err := c.ConvertToDomains()
	if err != nil {
		return nil, err
	}

	if len(domains) == 0 {
		return nil, nil
	}

	var lines []string
	for _, upstream := range upstreams {
		line := buildUpstreamLine(domains, upstream, subdomainsOnly)
		lines = append(lines, line)
	}

	return lines, nil
}

// detectFormat 自动检测文件格式
func (c *DomainListConverter) detectFormat() (DomainListFormat, error) {
	file, err := os.Open(c.SourceFile)
	if err != nil {
		return FormatPlain, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	dnsmasqCount := 0

	for scanner.Scan() && lineCount < 10 {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// GFWList: [AutoProxy ...]
		if strings.Contains(line, "[AutoProxy") {
			return FormatGFWList, nil
		}

		// Clash: payload: 或 - DOMAIN,
		if strings.Contains(line, "payload:") ||
			strings.HasPrefix(line, "- DOMAIN,") ||
			strings.HasPrefix(line, "- DOMAIN-SUFFIX,") {
			return FormatClash, nil
		}

		// Dnsmasq: server=/domain/dns
		if strings.HasPrefix(line, "server=/") {
			dnsmasqCount++
		}

		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return FormatPlain, fmt.Errorf("read file: %w", err)
	}

	if dnsmasqCount > 0 {
		return FormatDnsmasq, nil
	}

	return FormatPlain, nil
}

// convertPlain 转换纯文本格式
func (c *DomainListConverter) convertPlain() ([]string, error) {
	file, err := os.Open(c.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var domains []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 移除行内注释
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		if line != "" {
			// 保留通配符格式 (*.example.com)
			domains = append(domains, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return domains, nil
}

// convertDnsmasq 转换dnsmasq格式
// 格式: server=/domain.com/114.114.114.114
// 提取: domain.com
func (c *DomainListConverter) convertDnsmasq() ([]string, error) {
	file, err := os.Open(c.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var domains []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 server=/domain/dns
		if !strings.HasPrefix(line, "server=/") {
			continue
		}

		line = strings.TrimPrefix(line, "server=/")
		parts := strings.Split(line, "/")
		if len(parts) >= 2 {
			domain := strings.TrimSpace(parts[0])
			if domain != "" {
				domains = append(domains, domain)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return domains, nil
}

// convertGFWList 转换GFWList格式
// GFWList是Base64编码的规则列表
func (c *DomainListConverter) convertGFWList() ([]string, error) {
	data, err := os.ReadFile(c.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Base64解码
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(string(decoded)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行、注释和特殊标记
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		domain := extractDomainFromGFWRule(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	return domains, nil
}

// convertClash 转换Clash规则格式
func (c *DomainListConverter) convertClash() ([]string, error) {
	data, err := os.ReadFile(c.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// 尝试解析为YAML
	var clashRules struct {
		Payload []string `yaml:"payload"`
	}

	if err := yaml.Unmarshal(data, &clashRules); err != nil {
		// 如果不是YAML，尝试按行解析
		return c.convertClashPlain(string(data))
	}

	var domains []string
	for _, rule := range clashRules.Payload {
		domain := extractDomainFromClashRule(rule)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	return domains, nil
}

// convertClashPlain 转换纯文本Clash规则
func (c *DomainListConverter) convertClashPlain(content string) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		domain := extractDomainFromClashRule(line)
		if domain != "" {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	return domains, nil
}

// extractDomainFromGFWRule 从GFWList规则提取域名
func extractDomainFromGFWRule(rule string) string {
	// 移除规则前缀
	rule = strings.TrimPrefix(rule, "||")
	rule = strings.TrimPrefix(rule, "|")
	rule = strings.TrimPrefix(rule, "@@||")
	rule = strings.TrimPrefix(rule, "@@|")
	rule = strings.TrimPrefix(rule, "http://")
	rule = strings.TrimPrefix(rule, "https://")

	// 移除路径、参数、端口
	if idx := strings.IndexAny(rule, "/*?:"); idx >= 0 {
		rule = rule[:idx]
	}

	// 移除通配符
	rule = strings.TrimPrefix(rule, "*.")
	rule = strings.TrimSuffix(rule, "^")
	rule = strings.TrimSpace(rule)

	// 验证
	if rule == "" || strings.Contains(rule, " ") || isIPAddress(rule) {
		return ""
	}

	return rule
}

// extractDomainFromClashRule 从Clash规则提取域名
func extractDomainFromClashRule(rule string) string {
	rule = strings.TrimPrefix(rule, "- ")
	rule = strings.TrimSpace(rule)

	parts := strings.Split(rule, ",")
	if len(parts) < 2 {
		return ""
	}

	ruleType := strings.TrimSpace(parts[0])
	domain := strings.TrimSpace(parts[1])

	// 只处理 DOMAIN 和 DOMAIN-SUFFIX
	if ruleType == "DOMAIN" || ruleType == "DOMAIN-SUFFIX" {
		return domain
	}

	return ""
}

// isIPAddress 简单判断是否是IP地址
func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
