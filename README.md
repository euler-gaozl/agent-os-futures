# Agent OS 期货研究工作台

独立的中国期货基本面研究 Web 与 Go BFF，默认端口 **8098**，服务身份为 `futures`。业务用户通过研究入口调用已发布的 Agent，不需要接触 Skill、MCP、提示词或模型配置。

## 研究与服务边界

AI 负责收集、校验、分类、留存和客观变化反馈；预测、方向与交易决定由用户负责。资料按具体日历区间组织，实绩、计划和第三方预测分开。当前尚未完成全品种资料覆盖或真实研究质量验收。

BFF 只通过 HTTP 访问发布的期货 Agent 与本应用任务，不直接访问数据库，没有迁移命令，不提供管理工具或凭证转发。用户鉴权使用 `BUSINESS_TOKEN`（兼容独立的 `ADMIN_TOKEN` 回退），服务调用使用另一枚 `SERVICE_TOKEN`。本地开发模式仍检查 Host、Origin 和写请求标记；密钥不进入前端。

## 已有功能

- 发起研究、查看进度、结果和留存阅读记录。失败、取消或效果不明的任务即使没有报告，也能查看已经保存的资料；这不会触发重试或把任务改成完成。
- `source://document/<id>` 只打开本任务范围内实际留存的阅读回执，不用当前知识库或其他任务替代历史来源。
- 新任务固定分类日期 `as_of` 和 `futures-evidence-v1` 输出契约；分类日期不证明资料在历史当天已经公开。旧 Markdown 结果仍可阅读。
- 只读报告检查核对结构、日期、存量/流量/累计量/产能/比率/未履约承诺分类、计划与实绩、有限的同口径比较，以及与留存原文的逐字引文匹配。
- 导出保留原始模型结果、哈希、校验版本及来源标签。检查未通过不会改写历史结果或隐藏可定位的引文。

`GET /api/tasks/{id}/report-check` 限制为 96 KiB JSON、40 条事实、30 组比较、8 个问题；来源读取最多 48 次请求、256 KiB 正文、15 秒及每文档前 8 条回执。超限或不支持的表达标为待复核。规则不能替代通用语义验证、来源真实性验证或独立交叉核验。

## Agent 内的草稿检查工具

`POST /mcp/research` 提供只读 `check_research_draft_v2`：输入有界 JSON 草稿和 `as_of`，不接收任务 ID 或 URL，不请求外部服务、不写数据库。完整 MCP 返回限制为 5,900 UTF-8 字节，低于 Runtime 的 6,000 字节归档阈值；保留路径、问题代码、总数与截断标记。相同代码的解释完全一致时才合并解释，不同解释分别保留。

此接口仅接受本服务的 `SERVICE_TOKEN`。需在 Registry 单独保存凭证，在 Runtime/ToolHub 配置精确主机与端口白名单，完成发现、检查、启用，再绑定固定版本 Agent。启动服务不会自动完成这些管理操作。旧任务保持原绑定，最终网页检查仍使用该任务的真实阅读回执。

提交提示要求至多两次草稿检查，这是 Agent 指令而非新的硬性工具配额，实际执行受 Runtime 预算约束。真实反馈修订闭环目前仍受 Harness 上下文、预算和输出上限缺陷影响，见 [失败与后续修复记录](https://github.com/euler-gaozl/agent-os/blob/main/docs/research-feedback-20261014.md)。

## 构建与部署

需要 Go 1.27.2、Node 24.19.0、pnpm 11.25.0；固定 SDK rc.2 和 `vendor` 支持独立构建。

```powershell
$env:GOWORK='off'; $env:GOPROXY='off'
./scripts/Build.ps1
pnpm --dir web install --frozen-lockfile --ignore-scripts
pnpm --dir web test
pnpm --dir web run build
./scripts/Package-Web.ps1 -Destination <新的绝对路径>
./scripts/Test-RelocatedPackage.ps1 -Package <发布包路径>
```

使用仓库外的 [服务配置](deploy/service.example.json)，以 `--config` 启动；`FUTURES_WEB_DIR` 指向源码构建的 `web/dist` 或发布包的 `web`。健康检查读取 `AGENTOS_CONFIG`。先安装提供应用证据接口的 Runtime；旧 Runtime 只能显示报告并明确提示来源接口不可用。

BFF 和 Web 必须一起升级/回滚，保持相同 HTTP 契约。就绪检查不证明所有下游可用。Actions 日常关闭；独立模拟测试与实际安装、付费模型、研究质量分别验收。[历史实现细节](docs/history/service-notes-20261014.en.md) 保留原始记录。
