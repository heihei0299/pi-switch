# 08: config 路径解析与原子写收敛

**What to build:** config 的路径解析与原子写盘各只剩一份实现，且所有入口保存行为一致。端到端行为：`provider use` / `provider delete` 等 CLI 保存配置时，与 WebUI 一样应用保存迁移（写回文件不再残留旧字段）；TUI 切换 profile 的保存行为与 WebUI 一致，同一份配置不因入口不同而漂移；新增入口自动继承 `PI_SWITCH_CONFIG` 支持。

**实现要点**（spec D5，行为修正被接受）：

- `internal/config` 新增 `ResolvePath()`：`PI_SWITCH_CONFIG` > `~/.pi-switch/config.json` > `/tmp` 回退
- `internal/config` 新增 `SaveAtPath(cfg, path)`：内部调用既有的 `MigratedForSave` + 临时文件 rename 原子写
- server / tui / cmd 三处调用点改用新函数，并删除各自旧实现（当前为路径解析 4 份、写盘 3 份）

**新增测试**：一个 config 测试验证 `SaveAtPath` 应用迁移、原子写、生成合法 config shape；沿用 `internal/config/save_migration_test.go` 的既有风格。TUI 测试中 `saveConfig(cfg, path)` 调用点改为 `config.SaveAtPath`。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] `internal/config` 中 `ResolvePath()` / `SaveAtPath(cfg, path)` 存在并被 server、tui、cmd 三处调用
- [x] 旧实现全部删除：tui 的 `configPath`/`saveConfig`、cmd 的 `defaultConfigPath`/`defaultConfigDir`/`envOrDefault`/`saveConfigInner`；server 侧保留 `configPath`/`saveConfig` 签名不变，但实现体改为转发（二者是 spec D2 的 kernel 清单符号，域文件仍经它们访问 config）
- [x] 保存行为一致：TUI 与 CLI 保存后写回文件不残留旧字段（与 WebUI 同口径）
- [x] 路径解析一致：三处入口对 `PI_SWITCH_CONFIG`、默认 home、`/tmp` 回退的行为完全相同
- [x] 新测试覆盖：`SaveAtPath` 应用保存迁移、原子写、产物通过 config 解析成为合法 shape
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包）
- [x] 不新增 config 读缓存（architecture-review A6 属 out of scope）

## 实施记录

- 提交：`9d2bc0a`
- `internal/config/config.go` 新增 `ResolvePath()` 与 `SaveAtPath(cfg, path)`，紧随 `MigratedForSave`：
  - `ResolvePath`：`PI_SWITCH_CONFIG` > `~/.pi-switch/config.json` > `/tmp/pi-switch-config.json`
  - `SaveAtPath`：内部先 `MigratedForSave`，建父目录，`MarshalIndent` + 尾随换行，临时文件 + rename
- 行为修正（spec D5 接受）：**TUI 与 CLI 保存时开始应用保存迁移**。此外三处旧实现的分歧也已消除——旧 `server.configPath` 只判 `err != nil`，home 为空字符串时会拼出 `/.pi-switch/config.json`；新实现统一判 `err != nil || home == ""`，避免在根目录写文件
- 调用点收敛：cmd 4 处 → `config.ResolvePath()`；tui 2 处 → `config.ResolvePath()`、保存 → `config.SaveAtPath`；server 的 `configPath`/`saveConfig` 改为转发
- 测试：新增 `internal/config/save_at_path_test.go`（6 个用例：迁移应用、非法 ConversationSource 归一、原子写无残留临时文件、覆盖既有文件、`ResolvePath` 的环境变量优先与 home 回退）；`internal/tui` 的 4 处 `saveConfig(cfg, path)` 测试调用改为 `config.SaveAtPath`
- 随改动清理 import：server.go 的 `encoding/json`、tui/model.go 的 `encoding/json`/`os`/`path/filepath`
- 验收命令：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出
