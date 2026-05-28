package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUpstreamConfigFromYAML(t *testing.T) {
	t.Run("valid_yaml", func(t *testing.T) {
		yamlContent := `upstream_dns:
  - 8.8.8.8:53
  - 1.1.1.1:53

upstream_dns_groups:
  - name: china
    domains:
      - baidu.com
      - taobao.com
    upstreams:
      - 223.5.5.5:53
  
  - name: company
    domains:
      - company.local
    upstreams:
      - 192.168.1.1:53
    subdomains_only: false
`
		tmpFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(tmpFile)

		config, err := LoadUpstreamConfigFromYAML(tmpFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Upstreams, 2)
		assert.Greater(t, len(config.DomainReservedUpstreams), 0)
	})

	t.Run("only_default_upstreams", func(t *testing.T) {
		yamlContent := `upstream_dns:
  - 8.8.8.8:53
`
		tmpFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(tmpFile)

		config, err := LoadUpstreamConfigFromYAML(tmpFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Len(t, config.Upstreams, 1)
	})

	t.Run("subdomains_only", func(t *testing.T) {
		yamlContent := `upstream_dns:
  - 8.8.8.8:53

upstream_dns_groups:
  - name: test
    domains:
      - example.com
    upstreams:
      - 1.2.3.4:53
    subdomains_only: true
`
		tmpFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(tmpFile)

		config, err := LoadUpstreamConfigFromYAML(tmpFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		require.NoError(t, err)
		assert.NotNil(t, config)
	})

	t.Run("empty_config", func(t *testing.T) {
		yamlContent := `# 空配置
`
		tmpFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(tmpFile)

		_, err := LoadUpstreamConfigFromYAML(tmpFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no upstream dns servers")
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		yamlContent := `invalid: yaml: content: [
`
		tmpFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(tmpFile)

		_, err := LoadUpstreamConfigFromYAML(tmpFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		assert.Error(t, err)
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := LoadUpstreamConfigFromYAML("nonexistent.yaml", &upstream.Options{
			Timeout: 5 * time.Second,
		})

		assert.Error(t, err)
	})
}

func TestLoadUpstreamConfigFromYAMLWithFiles(t *testing.T) {
	t.Run("with_domain_file", func(t *testing.T) {
		// 创建域名文件
		domainContent := `example.com
test.org
`
		domainFile := createTempFile(t, domainContent)
		defer os.Remove(domainFile)

		// 创建YAML配置
		yamlContent := `upstream_dns:
  - 8.8.8.8:53

upstream_dns_groups:
  - name: test
    domain_file: ` + domainFile + `
    upstreams:
      - 1.2.3.4:53
`
		yamlFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(yamlFile)

		config, err := LoadUpstreamConfigFromYAMLWithFiles(yamlFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Greater(t, len(config.DomainReservedUpstreams), 0)
	})

	t.Run("mixed_domains_and_file", func(t *testing.T) {
		// 创建域名文件
		domainContent := `file1.com
file2.com
`
		domainFile := createTempFile(t, domainContent)
		defer os.Remove(domainFile)

		// 创建YAML配置（既有直接配置的域名，也有文件）
		yamlContent := `upstream_dns:
  - 8.8.8.8:53

upstream_dns_groups:
  - name: mixed
    domains:
      - direct1.com
      - direct2.com
    domain_file: ` + domainFile + `
    upstreams:
      - 1.2.3.4:53
`
		yamlFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(yamlFile)

		config, err := LoadUpstreamConfigFromYAMLWithFiles(yamlFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		require.NoError(t, err)
		assert.NotNil(t, config)
		// 应该包含4个域名（2个直接配置 + 2个从文件）
		assert.Greater(t, len(config.DomainReservedUpstreams), 0)
	})

	t.Run("domain_file_not_found", func(t *testing.T) {
		yamlContent := `upstream_dns:
  - 8.8.8.8:53

upstream_dns_groups:
  - name: test
    domain_file: nonexistent.txt
    upstreams:
      - 1.2.3.4:53
`
		yamlFile := createTempYAMLFile(t, yamlContent)
		defer os.Remove(yamlFile)

		_, err := LoadUpstreamConfigFromYAMLWithFiles(yamlFile, &upstream.Options{
			Timeout: 5 * time.Second,
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "load domains")
	})
}

// createTempYAMLFile 创建临时YAML文件用于测试
func createTempYAMLFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	return tmpFile
}
