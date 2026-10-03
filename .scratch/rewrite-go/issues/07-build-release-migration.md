## Question

Go 重写后的构建、发布与数据迁移策略如何设计？

pi-switch 现有通过 napi-rs 交叉编译为 `.node` 二进制，预构建多平台，通过 npm 发布（`@heihei0299/pi-switch`）。数据位于 `~/.pi-switch/`（config.json、SQLite DB、webui_password 等）。Go 重写后需改为 `go build` 交叉编译，发布方式与数据兼容需决策。

需要决策：
1. 构建：`go build` 交叉编译矩阵（linux/darwin/windows × amd64/arm64）？是否保留 `npm run build:webui && go build` 两阶段？
2. 发布：继续通过 npm 包装 Go 二进制（用户无感知）/ 改为直接发布 Go 二进制到 GitHub Releases / 两者并存？
3. 数据迁移：现有 `~/.pi-switch/config.json`、SQLite DB、备份文件是否零改动兼容？版本检测与自动迁移逻辑？
4. 守护进程：`pi-switch proxy/webui start --daemon` 的 pid 文件、端口管理在 Go 中如何实现（复用 daemon.rs 逻辑）？

## Type

grilling

## Status

resolved

## Answer

1. **构建**: 两阶段 `npm run build:webui && go build`，Go 交叉编译矩阵 linux/darwin/windows × amd64/arm64，modernc/sqlite 纯 Go 无 CGO，`go build -ldflags -s -w`。
2. **发布**: 继续 npm 包装 Go 二进制（`@heihei0299/pi-switch`），`bin/pi-switch.js` 按平台分发执行；GitHub Releases 作为补充（可选），首版先保证 npm 路径。
3. **数据迁移**: `~/.pi-switch/` 零改动兼容（config.json、SQLite DB、webui_password、备份），Go 启动时检测 version 字段，旧字段自动迁移后写回。
4. **守护进程**: 复用 daemon.rs 的 pid 文件与端口管理逻辑到 Go（internal/daemon），`pi-switch proxy/webui start --daemon --host --port` 语义保持。
