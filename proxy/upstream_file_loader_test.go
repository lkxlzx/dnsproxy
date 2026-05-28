package proxy

import (
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempDomainFile creates a temporary domain list file for testing
func createTempDomainFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "domains-*.txt")
	require.NoError(t, err)
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)
	return tmpFile.Name()
}

func TestLoadUpstreamConfigFromFileSimple(t *testing.T) {
	domainContent := `example.com
test.org
another.com
`
	domainFile := createTempDomainFile(t, domainContent)
	defer os.Remove(domainFile)

	upstreamAddrs := []string{"1.2.3.4:53"}
	defaultUpstreams := []string{"8.8.8.8:53"}

	config, err := LoadUpstreamConfigFromFileSimple(
		domainFile,
		upstreamAddrs,
		defaultUpstreams,
		&upstream.Options{},
	)

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.NotNil(t, config.Upstreams)
	assert.NotNil(t, config.DomainReservedUpstreams)
}

func TestLoadUpstreamConfigFromFiles(t *testing.T) {
	// 创建中国域名列表
	chinaDomains := `baidu.com
taobao.com
qq.com
`
	chinaFile := createTempDomainFile(t, chinaDomains)
	defer os.Remove(chinaFile)

	// 创建国际域名列表
	intlDomains := `google.com
youtube.com
facebook.com
`
	intlFile := createTempDomainFile(t, intlDomains)
	defer os.Remove(intlFile)

	groups := []DomainGroupConfig{
		{
			GroupName:      "china",
			DomainFile:     chinaFile,
			Upstreams:      []string{"223.5.5.5:53"},
			SubdomainsOnly: false,
		},
		{
			GroupName:      "international",
			DomainFile:     intlFile,
			Upstreams:      []string{"8.8.8.8:53"},
			SubdomainsOnly: false,
		},
	}

	defaultUpstreams := []string{"1.1.1.1:53"}

	config, err := LoadUpstreamConfigFromFiles(
		groups,
		defaultUpstreams,
		&upstream.Options{},
	)

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.NotNil(t, config.Upstreams)
	assert.NotNil(t, config.DomainReservedUpstreams)
}

func TestLoadUpstreamConfigFromFiles_WithSubdomainsOnly(t *testing.T) {
	domainContent := `example.com
test.org
`
	domainFile := createTempDomainFile(t, domainContent)
	defer os.Remove(domainFile)

	groups := []DomainGroupConfig{
		{
			GroupName:      "subdomains",
			DomainFile:     domainFile,
			Upstreams:      []string{"1.2.3.4:53"},
			SubdomainsOnly: true,
		},
	}

	config, err := LoadUpstreamConfigFromFiles(
		groups,
		[]string{"8.8.8.8:53"},
		&upstream.Options{},
	)

	require.NoError(t, err)
	require.NotNil(t, config)
}

func TestLoadUpstreamConfigFromFiles_MultipleFormats(t *testing.T) {
	// Plain format
	plainContent := `example.com
test.org
`
	plainFile := createTempDomainFile(t, plainContent)
	defer os.Remove(plainFile)

	// Dnsmasq format
	dnsmasqContent := `server=/baidu.com/114.114.114.114
server=/taobao.com/223.5.5.5
`
	dnsmasqFile := createTempDomainFile(t, dnsmasqContent)
	defer os.Remove(dnsmasqFile)

	groups := []DomainGroupConfig{
		{
			GroupName:        "plain",
			DomainFile:       plainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:        []string{"1.2.3.4:53"},
		},
		{
			GroupName:        "dnsmasq",
			DomainFile:       dnsmasqFile,
			DomainFileFormat: FormatDnsmasq,
			Upstreams:        []string{"223.5.5.5:53"},
		},
	}

	config, err := LoadUpstreamConfigFromFiles(
		groups,
		[]string{"8.8.8.8:53"},
		&upstream.Options{},
	)

	require.NoError(t, err)
	require.NotNil(t, config)
}

func TestBuildUpstreamLine(t *testing.T) {
	t.Run("normal_domains", func(t *testing.T) {
		domains := []string{"example.com", "test.org"}
		line := buildUpstreamLine(domains, "1.2.3.4:53", false)
		assert.Equal(t, "[/example.com/test.org/]1.2.3.4:53", line)
	})

	t.Run("subdomains_only", func(t *testing.T) {
		domains := []string{"example.com", "test.org"}
		line := buildUpstreamLine(domains, "1.2.3.4:53", true)
		assert.Equal(t, "[/*.example.com/*.test.org/]1.2.3.4:53", line)
	})

	t.Run("single_domain", func(t *testing.T) {
		domains := []string{"example.com"}
		line := buildUpstreamLine(domains, "8.8.8.8:53", false)
		assert.Equal(t, "[/example.com/]8.8.8.8:53", line)
	})
}
