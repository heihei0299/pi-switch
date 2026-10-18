## Question

Go 重写的配置系统应如何设计？保留 JSON 还是迁移到 YAML？如何处理现有用户的配置兼容？

pi-switch 现有配置使用 JSON（~/.pi-switch/config.json），包含 provider profiles、settings、failover 链等。CLIProxyAPI 使用 YAML（config.yaml），结构更丰富（插件、管理 API、OAuth 账户、并发控制等）。

需要决策：
1. 配置格式：保留 JSON / 迁移到 YAML / 支持两者？
2. 配置结构：是否重新设计 schema 以匹配 CLIProxyAPI 的更分层结构（providers、accounts、settings 分离）？
3. 向后兼容：现有 config.json 是否需要自动迁移？如何检测版本差异？
4. 热重载：现有 pi-switch 支持 per-request 热重载 config.json，Go 实现中如何保留？
5. 配置校验：现有 validate_config() 功能如何迁移？

## Type

grilling

## Status

resolved

## Answer

1. **配置格式**: 保留 JSON 为主，Go 默认读写 JSON。内部兼容读取 YAML（如用户提供 config.yaml），但默认输出 JSON。不增加现有用户迁移成本。
2. **配置结构**: 保留现有 schema 核心字段（profiles, settings, current, version），Go 中重新建模但保持字段名和结构兼容。新增字段可扩展。
3. **向后兼容**: Go 启动时读取现有 config.json；保存时写回 JSON。旧版本字段（如 injectOpenCodeAttribution）自动迁移到 conversationSource。
4. **热重载**: per-request 读取，保持现有 Rust 行为。简单可靠，无需 fsnotify 依赖。
5. **配置校验**: Go 中用 struct tag 校验 + 自定义校验逻辑，加载配置时统一收集并返回错误列表。
