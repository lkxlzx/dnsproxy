package proxy

import (
	"fmt"
)

// LoadDomainsFromFileWithFormat 从文件加载域名，支持多种格式
// 这是一个兼容性包装函数，内部使用 DomainListConverter
func LoadDomainsFromFileWithFormat(filepath string, format DomainListFormat) ([]string, error) {
	converter := NewDomainListConverter(filepath, format)
	return converter.ConvertToDomains()
}

// loadDomainsFromFile 从纯文本文件加载域名（兼容性函数）
func loadDomainsFromFile(filepath string) ([]string, error) {
	return LoadDomainsFromFileWithFormat(filepath, FormatPlain)
}

// LoadDomainsFromURL 从URL加载域名列表（支持HTTP/HTTPS）
func LoadDomainsFromURL(url string, format DomainListFormat) ([]string, error) {
	// TODO: 实现HTTP下载功能
	return nil, fmt.Errorf("URL loading not implemented yet")
}
