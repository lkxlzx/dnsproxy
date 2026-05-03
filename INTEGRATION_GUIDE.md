# Upstream Groups 集成指南

本文档说明如何将 Upstream Groups 功能集成到现有的 dnsproxy 项目中。

## 已创建的文件

### 核心代码文件

1. **proxy/upstreamgroup.go**
   - 核心数据结构和功能
   - `UpstreamGroup`: 上游组定义
   - `UpstreamGroupConfig`: 组配置管理
   - 组的添加、验证、查询等功能

2. **proxy/upstreamgroup_parser.go**
   - 配置解析功能
   - 支持 YAML 格式解析
   - 支持文本格式解析
   - `ParseUpstreamGroups()`: YAML 解析
   - `ParseUpstreamGroupsFromLines()`: 文本解析

3. **proxy/upstreamgroup_internal_test.go**
   - 单元测试
   - 测试核心功能
   - 验证错误处理

4. **proxy/upstreamgroup_example_test.go**
   - 示例代码
   - 演示如何使用 API
   - 可作为文档参考

### 配置文件示例

5. **config-groups.yaml.example**
   - 完整的 YAML 配置示例
   - 包含多种使用场景
   - 详细的注释说明

6. **groups.txt.example**
   - 简单的文本格式示例
   - 易于手动编辑
   - 适合快速配置

### 文档文件

7. **UPSTREAM_GROUPS.md**
   - 完整的功能文档
   - 详细的使用说明
   - 配置参数说明
   - 故障排查指南

8. **UPSTREAM_GROUPS_QUICKSTART.md**
   - 快速入门指南
   - 5 分钟上手
   - 常见场景示例

9. **INTEGRATION_GUIDE.md** (本文件)
   - 集成指南
   - 实现步骤说明

## 集成步骤

### 第 1 步：更新配置结构

需要修改 `internal/cmd/config.go`，添加上游组配置支持：

```go
// 在 configuration 结构体中添加
type configuration struct {
    // ... 现有字段 ...
    
    // UpstreamGroupsFile is the path to upstream groups configuration file
    UpstreamGroupsFile string `yaml:"upstream-groups-file"`
    
    // UpstreamGroups is the inline upstream groups configuration
    UpstreamGroups *proxy.UpstreamGroupsSpec `yaml:"upstream-groups"`
}
```

### 第 2 步：添加命令行参数

需要修改 `internal/cmd/args.go`，添加命令行参数：

```go
// 在 commandLineOptions 数组中添加
const (
    // ... 现有索引 ...
    upstreamGroupsFileIdx
)

var commandLineOptions = []*commandLineOption{
    // ... 现有选项 ...
    upstreamGroupsFileIdx: {
        description: "Path to upstream groups configuration file.",
        long:        "upstream-groups-file",
        short:       "",
        valueType:   "path",
    },
}
```

### 第 3 步：更新 Proxy 配置

需要修改 `proxy/config.go`，在 `Config` 结构体中添加：

```go
type Config struct {
    // ... 现有字段 ...
    
    // UpstreamGroupConfig is the upstream groups configuration.
    // If set, it will be used instead of UpstreamConfig for group-based routing.
    UpstreamGroupConfig *UpstreamGroupConfig
}
```

### 第 4 步：实现组选择逻辑

需要修改 `proxy/proxy.go` 或相关文件，在查询处理中集成组选择：

```go
// 在处理 DNS 查询时
func (p *Proxy) selectUpstreams(req *dns.Msg) []upstream.Upstream {
    if p.UpstreamGroupConfig != nil {
        // 使用组配置
        domain := req.Question[0].Name
        group, err := p.UpstreamGroupConfig.GetGroupForDomain(domain)
        if err == nil && group != nil {
            return group.Upstreams
        }
    }
    
    // 回退到原有逻辑
    return p.UpstreamConfig.getUpstreamsForDomain(domain)
}
```

### 第 5 步：添加配置加载逻辑

需要修改 `internal/cmd/proxy.go`，添加组配置加载：

```go
func loadUpstreamGroups(conf *configuration) (*proxy.UpstreamGroupConfig, error) {
    opts := &upstream.Options{
        Bootstrap: conf.BootstrapDNS,
        Timeout:   time.Duration(conf.Timeout),
        // ... 其他选项 ...
    }
    
    // 优先使用文件配置
    if conf.UpstreamGroupsFile != "" {
        lines, err := readLinesFromFile(conf.UpstreamGroupsFile)
        if err != nil {
            return nil, err
        }
        return proxy.ParseUpstreamGroupsFromLines(lines, opts)
    }
    
    // 使用内联配置
    if conf.UpstreamGroups != nil {
        return proxy.ParseUpstreamGroups(conf.UpstreamGroups, opts)
    }
    
    return nil, nil
}
```

### 第 6 步：更新验证逻辑

需要修改 `proxy/config.go` 中的 `validateConfig()` 方法：

```go
func (p *Proxy) validateConfig() (err error) {
    // 如果使用组配置，验证组配置
    if p.UpstreamGroupConfig != nil {
        err = p.UpstreamGroupConfig.Validate()
        if err != nil {
            return fmt.Errorf("upstream groups: %w", err)
        }
        
        // 记录组信息
        p.UpstreamGroupConfig.LogGroupInfo(p.logger)
        
        return nil
    }
    
    // 原有验证逻辑
    err = p.UpstreamConfig.validate()
    // ...
}
```

## 向后兼容性

该实现保持了完全的向后兼容性：

1. **不影响现有配置**：如果不使用组功能，现有配置继续工作
2. **可选功能**：组配置是可选的，不是必需的
3. **回退机制**：如果组配置不可用，自动回退到原有逻辑
4. **独立模块**：组功能在独立的文件中实现，不修改核心逻辑

## 测试

### 运行单元测试

```bash
cd proxy
go test -v -run TestUpstreamGroup
```

### 运行示例测试

```bash
cd proxy
go test -v -run Example
```

### 集成测试

创建测试配置文件并启动：

```bash
# 使用 YAML 配置
./dnsproxy --config-path=config-groups.yaml.example --verbose

# 使用文本配置
./dnsproxy --upstream-groups-file=groups.txt.example -l 127.0.0.1 -p 5353 --verbose
```

测试查询：

```bash
# Linux/Mac
dig @127.0.0.1 -p 5353 google.com

# Windows
nslookup google.com 127.0.0.1
```

## 性能考虑

1. **组查找优化**：使用 map 进行 O(1) 查找
2. **上游复用**：相同地址的上游只创建一次
3. **缓存友好**：与现有缓存机制兼容
4. **内存效率**：只在需要时创建组配置

## 扩展建议

### 未来可能的增强

1. **动态重载**：支持运行时重新加载组配置
2. **健康检查**：定期检查上游服务器健康状态
3. **统计信息**：记录每个组的查询统计
4. **权重配置**：为组内服务器配置权重
5. **条件路由**：基于客户端 IP、时间等条件选择组
6. **API 接口**：提供 HTTP API 管理组配置

### 可选的高级特性

1. **组级缓存**：每个组独立的缓存配置
2. **组级限流**：每个组独立的速率限制
3. **自动故障转移**：自动切换到备用组
4. **负载监控**：实时监控组的负载情况

## 代码审查清单

在集成前，请确认：

- [ ] 所有新文件都有适当的包声明和导入
- [ ] 代码遵循项目的编码规范
- [ ] 所有公共 API 都有文档注释
- [ ] 单元测试覆盖主要功能
- [ ] 示例代码可以正常运行
- [ ] 配置文件示例格式正确
- [ ] 文档清晰易懂
- [ ] 向后兼容性得到保证
- [ ] 错误处理完善
- [ ] 日志输出合理

## 部署建议

### 开发环境

1. 使用详细日志：`--verbose`
2. 使用小的超时值快速测试
3. 先测试简单配置，再测试复杂场景

### 生产环境

1. 充分测试配置文件
2. 设置合理的超时和重试值
3. 启用缓存提高性能
4. 监控日志输出
5. 准备回退方案

## 获取帮助

如有问题：

1. 查看 [UPSTREAM_GROUPS.md](UPSTREAM_GROUPS.md) 完整文档
2. 查看 [UPSTREAM_GROUPS_QUICKSTART.md](UPSTREAM_GROUPS_QUICKSTART.md) 快速入门
3. 运行示例测试了解 API 用法
4. 在 GitHub 提交 Issue

## 贡献

欢迎贡献代码和文档改进！请遵循项目的贡献指南。
