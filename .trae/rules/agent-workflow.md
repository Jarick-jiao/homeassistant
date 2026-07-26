# Agent 工作流规则

> 本文件为项目级 Agent 规则，会被 Trae IDE 自动加载。任何对本仓库的修改必须遵守。

## 适用范围

`internal/`、`web/`、`scripts/`、`cmd/`、`mcp-servers/`、`docs/` 下的代码与配置修改均适用本规则。

## 强制收尾流程（顺序不可省）

每次完成一组代码修改后，必须按以下顺序执行收尾，**禁止跳步**：

1. `go build ./...` — 编译通过
2. `go vet ./...` — 静态检查通过
3. `git add -A` — 暂存全部改动
4. `git commit -m "<type>(<scope>): <subject>"` — 提交（Conventional Commits）
5. `git push` — 推送到远程仓库

> **关键约束：每次提交代码必须 push 到仓库。** 仅 commit 不 push 视为流程未完成。

## 中止条件

- `go build` 或 `go vet` 失败时，**禁止 commit**，必须先修复至全部通过。
- 修复后重新从第 1 步开始执行，不可仅补 commit。

## 纯文档/报告修改

仅涉及 Markdown、报告、说明类文件（`docs/`、`README.md`、`*.md`）的修改可跳过 `build`/`vet`，但仍需执行 `git add -A` → `git commit` → `git push`。

## Commit Message 规范

遵循 Conventional Commits：

- `type` ∈ `feat | fix | docs | refactor | chore | test`
- `scope` 可选，表示影响模块（如 `scheduler`、`store`、`web`）
- `subject` 简明描述改动

示例：

```
fix(store): data_source_config 增加唯一索引并清理历史重复行
feat(scheduler): 状态接口分页 + 任务启停与间隔编辑
docs: 补充 v3.9.13 修复报告
```

## 分任务提交

多个独立任务应分多次提交，每次提交对应一个完整任务，便于回溯与回滚。每提交一次即 push 一次。
