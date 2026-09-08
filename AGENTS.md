
## 快速上手

1. 读 `CONTEXT.md`（术语）——没有则跳过
2. 按行为路由表行动；未命中用 ask-matt 或直接澄清
3. 探索代码库：直接使用 `codegraph explore`（`codegrafh CLI`）；若无 `.codegraph/` 索引先执行 `codegraph init` 初始化，再 `explore`



## 行为路由
命中即行动，回复中简短声明所用 skill 或工具。
- 理解/定位 → `codegraph explore`
- 调研/原型 → `research` / `prototype`
- 修改/实现 → 简单低风险直接执行：理解现状 → 最小修改 → 相关验证；有实际改动且验证通过时按一个用户请求执行一次 `git commit`；测试先行、TDD 或集成测试 → `tdd`；bug、失败、异常或性能问题 → `diagnose-fix`；其它中大型修改 → 先澄清范围、验收和验证方式，再按项目流程执行
- 审查/设计 → `code-review` / `grilling` / `domain-modeling`
- 无法归类 → 直接澄清

## 分文件

- Issue tracker → `docs/agents/issue-tracker.md`；Triage labels → `docs/agents/triage-labels.md`；Domain docs → `docs/agents/domain.md`
- 术语表 → `CONTEXT.md`

## CodeGraph

理解/定位代码**必须**使用 `codegrafh CLI`，直接优于 grep/find/读文件——一次调用拿到相关符号逐字源码与调用路径：

- **CLI**：`codegraph explore "<符号名或问题>"` 一次回答大部分代码问题——相关符号的逐字源码 + 调用路径（含 grep 追不上的动态分派跳转）。在 query 中指名文件/符号即可读取其带行号的当前源码，默认 `maxFiles: 12` 覆盖跨 5-8 文件调用链。
- **初始化**：若根目录无 `.codegraph/`，先执行 `codegraph init` 初始化索引，再 `explore`；已有索引直接 `explore`（硬判定，不回退 explore 子代理）。
- **已读等价**：返回体含完整源码块的文件视为已 `Read`，不再重复 `read`；仅返回调用路径片段时补一次带行号 `read`。
- **跨仓/子项目**：仅当探索第二代码库或 monorepo 子项目（根无索引但子目录有）时显式传 `projectPath`。
与 `research`（后台调研产出 Markdown 文件）分工：`codegraph explore` 为代码定位唯一首选，`research` 仅用于需产出调研文档的后台任务。
