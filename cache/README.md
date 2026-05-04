# Cache 目录

这个目录用于存储域名列表的缓存文件。

## 用途

当你在配置文件中使用 `domains_lists` 功能时，dnsproxy 会：

1. 从远程 URL 下载域名列表
2. 转换为标准 YAML 格式
3. 保存到这个目录中

## 自动管理

- **自动创建**：如果目录不存在，首次运行时会自动创建
- **自动下载**：首次运行时会下载所有配置的域名列表
- **自动更新**：如果启用 `auto_update`，会定期更新缓存文件
- **离线使用**：如果网络不可用，会使用已缓存的文件

## 示例

配置文件中的示例：

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china-domains.yaml  # 缓存到这个文件
    auto_update: true
    refresh_interval: 6h
    enabled: true
    format: dnsmasq
```

运行后，会在这个目录中创建 `china-domains.yaml` 文件。

## 文件格式

缓存文件使用 YAML 格式，包含：

```yaml
# 元数据
source: https://example.com/list.txt
format: dnsmasq
updated_at: 2026-05-04T12:00:00Z
domain_count: 1000

# 域名列表
domains:
  - example.com
  - test.com
  - "*.cn"
```

## 清理缓存

如果需要重新下载域名列表：

```bash
# 删除所有缓存文件
rm -f cache/*.yaml

# 或删除特定文件
rm -f cache/china-domains.yaml

# 重新启动 dnsproxy，会自动重新下载
./dnsproxy -c config.yaml
```

## 注意事项

1. **不要手动编辑缓存文件** - 它们会被自动覆盖
2. **可以提交到 Git** - 如果你想在离线环境使用
3. **定期清理** - 可以使用 `cleanup_interval` 配置自动清理过期文件
4. **备份重要列表** - 如果你有自定义的域名列表，建议备份

## 示例文件

- `example-converted.yaml` - 示例缓存文件，展示文件格式
