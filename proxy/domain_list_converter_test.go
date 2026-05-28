package proxy

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempFile creates a temporary file with the given content for testing
func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "domain-list-*.txt")
	require.NoError(t, err)
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)
	return tmpFile.Name()
}

func TestDomainListConverter_ConvertToDomains(t *testing.T) {
	t.Run("plain_format", func(t *testing.T) {
		content := `example.com
test.org
# comment
another.com
`
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		converter := NewDomainListConverter(tmpFile, FormatPlain)
		domains, err := converter.ConvertToDomains()
		require.NoError(t, err)
		assert.Equal(t, []string{"example.com", "test.org", "another.com"}, domains)
	})

	t.Run("dnsmasq_format", func(t *testing.T) {
		content := `server=/baidu.com/114.114.114.114
server=/taobao.com/223.5.5.5
`
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		converter := NewDomainListConverter(tmpFile, FormatDnsmasq)
		domains, err := converter.ConvertToDomains()
		require.NoError(t, err)
		assert.Equal(t, []string{"baidu.com", "taobao.com"}, domains)
	})

	t.Run("gfwlist_format", func(t *testing.T) {
		rules := `[AutoProxy 0.2.9]
||google.com
||youtube.com
`
		encoded := base64.StdEncoding.EncodeToString([]byte(rules))
		tmpFile := createTempFile(t, encoded)
		defer os.Remove(tmpFile)

		converter := NewDomainListConverter(tmpFile, FormatGFWList)
		domains, err := converter.ConvertToDomains()
		require.NoError(t, err)
		assert.Contains(t, domains, "google.com")
		assert.Contains(t, domains, "youtube.com")
	})

	t.Run("clash_format", func(t *testing.T) {
		content := `payload:
  - DOMAIN,google.com
  - DOMAIN-SUFFIX,youtube.com
`
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		converter := NewDomainListConverter(tmpFile, FormatClash)
		domains, err := converter.ConvertToDomains()
		require.NoError(t, err)
		assert.Contains(t, domains, "google.com")
		assert.Contains(t, domains, "youtube.com")
	})

	t.Run("auto_detect", func(t *testing.T) {
		content := `server=/example.com/114.114.114.114
`
		tmpFile := createTempFile(t, content)
		defer os.Remove(tmpFile)

		converter := NewDomainListConverter(tmpFile, FormatAuto)
		domains, err := converter.ConvertToDomains()
		require.NoError(t, err)
		assert.Equal(t, []string{"example.com"}, domains)
	})
}

func TestDomainListConverter_ConvertToUpstreamLines(t *testing.T) {
	content := `example.com
test.org
`
	tmpFile := createTempFile(t, content)
	defer os.Remove(tmpFile)

	converter := NewDomainListConverter(tmpFile, FormatPlain)
	lines, err := converter.ConvertToUpstreamLines([]string{"1.2.3.4:53"}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"[/example.com/test.org/]1.2.3.4:53"}, lines)
}
