package main

import (
	"fmt"
	"log"
	"os"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
	fmt.Println("=== 多格式域名列表加载示例 ===\n")

	// 示例1: 自动检测格式
	fmt.Println("1. 自动检测格式")
	demonstrateAutoDetect()

	// 示例2: 指定格式转换
	fmt.Println("\n2. 指定格式转换")
	demonstrateSpecificFormat()

	// 示例3: 多格式混合配置
	fmt.Println("\n3. 多格式混合配置")
	demonstrateMultipleFormats()

	// 示例4: 转换为上游配置行
	fmt.Println("\n4. 转换为上游配置行")
	demonstrateUpstreamLines()
}

func demonstrateAutoDetect() {
	// 创建测试文件
	dnsmasqContent := `server=/baidu.com/114.114.114.114
server=/taobao.com/223.5.5.5
server=/qq.com/119.29.29.29
`
	tmpFile := createTempFile("dnsmasq-test-*.conf", dnsmasqContent)
	defer os.Remove(tmpFile)

	// 自动检测格式
	converter := proxy.NewDomainListConverter(tmpFile, proxy.FormatAuto)
	domains, err := converter.ConvertToDomains()
	if err != nil {
		log.Fatalf("转换失败: %v", err)
	}

	fmt.Printf("  文件: %s\n", tmpFile)
	fmt.Printf("  检测到格式: Dnsmasq\n")
	fmt.Printf("  提取的域名: %v\n", domains)
}

func demonstrateSpecificFormat() {
	// Plain格式
	plainContent := `example.com
test.org
# 这是注释
another.com
`
	plainFile := createTempFile("plain-*.txt", plainContent)
	defer os.Remove(plainFile)

	converter := proxy.NewDomainListConverter(plainFile, proxy.FormatPlain)
	domains, err := converter.ConvertToDomains()
	if err != nil {
		log.Fatalf("转换失败: %v", err)
	}

	fmt.Printf("  Plain格式域名: %v\n", domains)
}

func demonstrateMultipleFormats() {
	// 创建不同格式的文件
	plainFile := createTempFile("plain-*.txt", "example.com\ntest.org\n")
	defer os.Remove(plainFile)

	dnsmasqFile := createTempFile("dnsmasq-*.conf", "server=/baidu.com/114.114.114.114\n")
	defer os.Remove(dnsmasqFile)

	clashContent := `payload:
  - DOMAIN,google.com
  - DOMAIN-SUFFIX,youtube.com
`
	clashFile := createTempFile("clash-*.yaml", clashContent)
	defer os.Remove(clashFile)

	// 配置多个分组
	groups := []proxy.DomainGroupConfig{
		{
			GroupName:        "plain-group",
			DomainFile:       plainFile,
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"1.2.3.4:53"},
		},
		{
			GroupName:        "dnsmasq-group",
			DomainFile:       dnsmasqFile,
			DomainFileFormat: proxy.FormatDnsmasq,
			Upstreams:        []string{"223.5.5.5:53"},
		},
		{
			GroupName:        "clash-group",
			DomainFile:       clashFile,
			DomainFileFormat: proxy.FormatClash,
			Upstreams:        []string{"8.8.8.8:53"},
		},
	}

	config, err := proxy.LoadUpstreamConfigFromFiles(
		groups,
		[]string{"1.1.1.1:53"}, // 默认上游
		&upstream.Options{},
	)

	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	fmt.Printf("  成功加载 %d 个分组\n", len(groups))
	fmt.Printf("  配置对象: %+v\n", config != nil)
}

func demonstrateUpstreamLines() {
	content := `baidu.com
taobao.com
qq.com
`
	tmpFile := createTempFile("domains-*.txt", content)
	defer os.Remove(tmpFile)

	converter := proxy.NewDomainListConverter(tmpFile, proxy.FormatPlain)
	
	// 转换为上游配置行
	lines, err := converter.ConvertToUpstreamLines(
		[]string{"223.5.5.5:53", "119.29.29.29:53"},
		false,
	)
	if err != nil {
		log.Fatalf("转换失败: %v", err)
	}

	fmt.Println("  生成的上游配置行:")
	for _, line := range lines {
		fmt.Printf("    %s\n", line)
	}
}

func createTempFile(pattern, content string) string {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		log.Fatalf("创建临时文件失败: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		log.Fatalf("写入文件失败: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		log.Fatalf("关闭文件失败: %v", err)
	}
	return tmpFile.Name()
}
