# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.11.0-rc1] - 2025-02-25

### Added

- **视觉屏幕自动化**：新增 device_screen_snapshot、device_screen_analyze、device_screen_wait 工具，支持基于 Vision 的「看屏决策」自动化流程；与 plan 模式结合，用户可先获得执行计划再确认执行；maxIterations 提升至 15 以支持多步截图→分析→点击→等待循环

### Fixed

- **摘要生成 provider 错误**：Engine.checkAndSummarize / summarizeConversation 此前始终使用 config 默认 provider（zai），现已改为使用用户选择的 chat_provider（如 qwen），避免用户选择 qwen 时仍向 z.ai 发请求

## [1.10.0-rc5] - 2025-02-25

### Fixed

- **设置页标签无法切换**：修复 `Invalid left-hand side in assignment` 语法错误，vision_provider 的 `?.checked = true` 改为先取元素再赋值

## [1.10.0-rc4] - 2025-02-25

### Fixed

- **会话标题生成 provider 错误**：GenerateSessionTitle 此前始终使用 config 默认 provider（zai），现已改为使用用户选择的 chat_provider（如 qwen），与主对话、memory、summary 行为一致

## [1.10.0-rc3] - 2025-02-24

### Added

- **vLLM 本地模型**：支持 vLLM 作为 AI 提供商（默认 http://localhost:8000/v1）
- **默认心跳任务 API**：`POST /api/afk/tasks/default-heartbeat` 一键创建每 30 分钟检查的 AIDriven 任务
- **ClawHub 技能市场兼容**：RemoteRegistry 支持 ClawHub 格式（tagline、homepage 映射）
- **扩充内置技能**：builtin_registry 从 2 个扩充到 6 个（calculator、weather、todo、web_search、file_ops、git_workflow）

### Changed

- **Adapter 配置健壮性**：token 为空时输出明确 warn 日志，提示所需环境变量

## [1.8.0-rc1] - 2025-02-24

### Added

- **Ollama 本地模型**：支持 Ollama 作为 AI 提供商，满足离线与隐私需求（阶段一）
- **Discord / Slack 适配器**：新增 Discord、Slack IM 适配器；Adapter 启动逻辑，Telegram/Matrix/Discord/Slack 在 main 中统一启动（阶段二）
- **心跳调度器**：AIDriven 任务按 CheckInterval 主动唤醒，Agent 可周期性检查待办并执行（阶段三）
- **默认技能市场**：BuiltinRegistry 内置技能索引，search_skills 开箱即用；CompositeRegistry 合并 builtin + remote（阶段四）

## [1.4.0-rc1] - 2025-02-24

### Added

- **Agent 协作工具**：sessions_list、sessions_history、sessions_send、sessions_spawn，支持跨会话协作
- **技能发现**：RemoteRegistry 客户端、search_skills 工具、install_skill 支持 registry_id，skills.registry_url 配置
- **安全沙箱**：security.sandbox 配置（mode: off/non_main）、主/非主会话判定（IsMainSession）、Docker 隔离 SandboxExecutor、工具白名单
- **Onboarding CLI**：cmd/onboard 交互式配置向导，数据库/AI 提供商/可选配置，生成 config.yaml 与 .env.example
- **Daemon 服务**：--install-daemon 生成 systemd/launchd 服务文件，deployments/systemd、deployments/launchd 模板
- Skills API：InstallSkill 支持从 URL 拉取（HTTPS）
- 新增 internal/sandbox.Whitelist、internal/skill/registry_client.go、deployments/docker/Dockerfile.sandbox

## [1.0.0-rc4] - 2025-02-24

### Added

- 接入 DeepSeek、MiniMax 中国版、MiniMax 国际版为可选聊天引擎
- MiniMax 分中国版 (api.minimax.chat) 与国际版 (api.minimaxi.com)

## [1.0.0-rc3] - 2025-02-24

### Fixed

- Qwen 图片生成空响应：添加 `tool_choice: "auto"` 鼓励工具调用；Qwen 在有工具时改用非流式首轮调用，规避流式模式下 tool_calls 兼容性问题

## [1.0.0-rc2] - 2025-02-24

### Fixed

- 图片生成空响应：当 LLM 执行工具（如 generate_image）后未返回文本时，使用工具执行结果作为响应内容，避免「后端返回空响应」

## [1.0.0] - 2025-02-24

### Added

- 版本号管理：`Version` 变量、`/api/health` 返回 version、`X-Marstaff-Version` 响应头
- CHANGELOG.md 与 README 版本信息、变更记录链接
