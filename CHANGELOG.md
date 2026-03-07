# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.17.0-rc2] - 2026-03-08

### Fixed

- **视频工作流 context 注入**：pipeline 执行时注入 `UserID` 与 `SessionID` 到 context，修复视频生成工具无法创建 AFK 异步任务的问题（`user_id and session_id not provided`）
- **工作流失败详情展示**：Chat 工作流面板在步骤或整体失败时展示 `step.error` / `pipeline.error`，便于排查「Generate scene videos」等失败原因

### Added

- **排查脚本**：`make debug-pipeline SESSION_ID=xxx` 可快速拉取当前会话的 pipeline 错误详情（需 jq）

## [1.17.0-rc1] - 2026-03-07

### Added

- **工作流专属页面**：新增 `/workflows` 页面，可查看所有视频工作流列表、状态、步骤进度与最终视频结果，支持按用户筛选与刷新
- **工作流查询能力**：系统提示增强，当用户询问「工作流 X 的状态」「工作流进度」时，Agent 会调用 `pipeline_status` 查询；`pipeline_list` 支持 `session_id` 参数，可列出当前会话的工作流

### Changed

- **导航入口**：Chat 与 AFK 页面增加「工作流」入口，便于从任意页面跳转至工作流列表

## [1.16.0-rc3] - 2026-03-07

### Fixed

- **CodingStats 迁移告警**：修复 `OutputTokens` 字段上错误的 `gorm default` tag，消除 Gateway 启动时 `failed to parse DEFAULT as default value for int` 告警，并恢复 `coding_stats` 表的正常迁移

## [1.16.0-rc2] - 2026-03-07

### Fixed

- **长视频工具误路由**：为多分镜视频场景补充更强的系统提示，并明确禁止使用 `pipeline_create` 手写 `ffmpeg/concat/scene` 流程，降低 30 秒故事视频再次误走通用工作流工具的概率
- **视频工作流工具护栏**：`pipeline_create` 现在会识别多分镜视频/拼接型请求并直接拒绝，提示改用 `video_story_workflow_create`，避免模型“嘴上说工作流，手上拼文本文件”
- **超时长单分镜兜底**：`video_story_workflow_create` 现在会拒绝超过单次模型上限（如 15 秒）的单分镜，并要求重新拆分 scenes，避免 30 秒故事被塞成一个 scene
- **PipelineStep 缺表**：Gateway 启动迁移现补上 `PipelineStep` 模型，修复 `/api/pipelines` 因 `pipeline_steps` 表不存在而报错的问题

## [1.16.0-rc1] - 2026-03-07

### Added

- **通用多分镜视频工作流**：新增 `video_story_workflow_create` 工具，支持将长故事视频拆成任意 N 段分镜，统一创建主工作流、并行生成多个视频子任务，并在全部完成后自动拼接为最终视频
- **聊天页工作流面板**：`/chat` 新增视频工作流状态卡片，可展示主工作流状态、当前阶段、子任务步骤与最终合成视频结果，并在整体完成或失败时弹出通知

### Changed

- **Pipeline 异步编排能力**：增强 parallel step，对 `tool.generate_video` 等异步任务走统一等待与结果聚合流程，自动回填 `video_urls` 并为后续 concat 步骤提供变量
- **多分镜视频指令策略**：系统提示新增规则，遇到需要拆分分镜并最终合成的长视频请求时，优先使用视频工作流工具，而不是直接单次 `generate_video`
- **工作流回归测试覆盖**：补充 `video_story_workflow_create`、`/api/pipelines?session_id=...` 和系统提示路由测试，降低长视频流程再次退化为“单段完成即整体完成”的风险
- **AFK 任务详情展示**：`/afk` 任务详情增加 result URL、错误信息与 workflow metadata 展示，便于查看子任务归属的 pipeline/step/subtask

### Fixed

- **提前宣告“视频已完成”**：修复长视频请求只完成第一个异步视频任务就被当作整体完成的问题；现在只有最终拼接成功后，工作流才会进入 completed
- **分镜任务语义误导**：工作流中的子任务完成通知改为“分镜完成”，避免与最终整片完成混淆
- **Pipeline 仓储时间更新兼容性**：`pipeline_repo` 不再依赖 `NOW()` SQL 表达式，避免在 sqlite 等环境下测试失败

## [1.15.0-rc4] - 2025-03-04

### Fixed

- **afk_create_oneoff_task 参数解析失败**：Gemini 流式返回时可能将 tool call arguments 拆成多块拼接（如 `{"name":"x"}{"command":"y"}`），导致 `invalid character '{' after top-level value`。现增加 `parseToolArguments` 容错解析，支持合并多个 JSON 对象；解析失败时记录 raw_args 便于排查

## [1.15.0-rc3] - 2025-03-04

### Fixed

- **挂机任务 firecrawl 结果文件为空**：firecrawl 使用 `-o` 将结果写入指定文件，stdout 几乎为空，导致上传的 .log 为 0 字节。现当 log 为空时，解析命令中的 `-o` 输出路径并优先上传该文件（如 `.firecrawl/potential_clients.json`），飞书通知中的链接将指向实际结果

### Added

- **浏览器自动化模式选择**：设置页新增「浏览器自动化」选项，支持「新开 Chrome」（默认）与「连接已有 Chrome」（带登录信息）。选择连接模式时需配置 CDP 端口，并提供 macOS/Linux/Windows 启动命令说明；后端提供端口检测接口，可验证端口是否被 Chrome 远程调试占用
- **环境变量管理**：设置页新增「环境变量」标签，支持单项添加/编辑/删除，以及 .env 格式批量导入。配置的变量会注入到 Agent 执行的所有命令中（run_command、挂机任务等）。批量导入时，文本中不存在的键会被删除
- **挂机任务结果上传 OSS**：一次性任务（firecrawl、npm install 等）完成后，若已配置 OSS，日志文件会自动上传到 OSS，飞书/邮件通知中发送可点击的 URL，不再仅显示本地路径
- **万相 2.6 完整特性支持**：实现阿里万相 2.6 模型的所有参数支持，包括音频 (audio, audio_url)、多镜头叙事 (shot_type)、提示词扩展 (prompt_extend)、水印 (watermark)、模板 (template) 等；支持 AI 从自然语言解析参数（如 "10秒的1080p竖屏视频"）
- **通用文件下载工具**：新增 `download_file` 工具，支持从任意 HTTP/HTTPS URL 下载文件到 session work_dir，下载后可用于 FFmpeg 等其他工具处理
- **AFK 页面聊天入口**：在 /afk 页面的任务详情中添加"进入聊天"按钮，点击可跳转到触发该任务的聊天会话
- **聊天内技能管理**：系统提示新增技能管理功能说明，用户可通过自然语言在聊天中查看、启用、禁用、搜索和安装技能

### Fixed

- **挂机任务 command_execution 结果误渲染为图片**：firecrawl 等一次性任务的 result_url 是本地日志文件路径，聊天 UI 此前将其当作图片 URL 渲染导致大量「图片加载失败」。现对 command_execution 类型及 .log/.json 等文件路径显示为文本块，提示用户前往 /afk 查看
- **afk_create_oneoff_task 装配**：修复 gateway 未调用 `SetupOneOffTasks` 导致 `afk_create_oneoff_task` 报 "one-off tasks not configured" 的问题，现已在启动时正确装配 sessionRepo、asyncNotifier、validator
- **oneoff exit 127**：OneOffRunner 改用 bash 登录 shell 执行命令，以加载用户 PATH（~/.nvm、/usr/local/bin 等）；工具描述建议使用 `npx firecrawl-cli` 替代 `firecrawl`
- **Gemini 400 INVALID_ARGUMENT**：修复工具调用后第二轮请求报 400 的问题，Gemini 不接受 assistant 消息中 content 为空且含 tool_calls 的情况，现对空 content 省略该字段
- **rules GetActive record not found**：改用 Find+Limit 替代 First，避免无活跃规则时 GORM 打印 "record not found" 日志
- **Gemini API 认证方式**：修复 Gemini 引擎报 "Missing Authorization header" 的问题，改为使用 `Authorization: Bearer` 头而非 URL 查询参数（Google OpenAI 兼容接口要求）
- **run_command 中 ~ 路径展开**：修复 `bash ~/script.sh` 中 `~` 被错误展开成用户主目录的问题，现在正确展开为 session work_dir
- **search_files 中 ~ 路径展开**：修复 `search_files path=~/Sites` 中 `~` 未展开导致搜索失败的问题
- **list_skills 显示已启用技能**：默认只显示已启用的技能，避免 AI 声称有未启用技能的能力；添加 `show_all=true` 参数查看所有技能
- **视频结果持久化**：修复 OSS 上传失败时视频 URL 未保存到数据库的问题，现在无论上传成功与否都会保存结果 URL
- **分辨率参数映射**：修复万相 2.6 分辨率参数硬编码问题，正确映射 720p → 1280*720、1080p → 1920*1080
- **时长限制修正**：万相 2.6 支持最高 15 秒视频，之前错误限制为 10 秒
- **记忆提取 JSON 截断**：记忆提取请求 MaxTokens 从 1000 提升至 4000；prompt 限制最多 8 条并排除任务ID等临时数据；解析失败时尝试从截断 JSON 中恢复已完整的记忆对象

### Changed

- **设置页布局优化**：设置项改为左侧边栏垂直排列，内容区宽度扩展至 1400px 铺满屏幕，解决标签切换处文字错乱问题
- **系统提示明确本地能力**：更新系统提示，明确说明这是本地 AI Agent 平台，工具/技能运行在本地而非云 AI 服务
- **工具描述增强**：技能管理工具 (list_skills, enable_skill, disable_skill, search_skills, install_skill) 添加中英文使用示例
- **afk_create_oneoff_task**：新增一次性挂机任务工具，支持 firecrawl、npm/yarn/pip 安装、ffmpeg、大型构建等耗时命令。Agent 优先使用此工具而非 run_command，任务在后台执行，完成后通过飞书/邮件/WebSocket 通知。主程序需调用 `AFKExecutor.SetupOneOffTasks(sessionRepo, asyncNotifier, validator)` 完成装配

## [1.13.0-rc1] - 2025-03-04

### Added

- **设置页 API Key 管理**：在设置页可配置各 Provider 的 API Key，写入数据库并覆盖配置文件
- **工具读取配置**：新增 `get_config` 工具，执行工具时通过 context 注入安全配置

## [1.12.0-rc1] - 2025-03-04

### Added

- **Playwright 浏览器自动化**：浏览器控制从 chromedp 迁移至 Playwright Node.js sidecar，通过 stdio JSON-RPC 2.0 通信；新增 `device_browser_snapshot` 返回页面可交互元素编号列表，`device_browser_click(ref)` 和 `device_browser_fill(ref, text)` 通过编号精准操作，替代原有「截图→VLM 猜坐标→tap」流程
- **按需启动与空闲回收**：Playwright sidecar 首次调用浏览器工具时自动启动，5 分钟无调用自动回收

### Removed

- **废弃 chromedp 相关工具**：device_browser_connect、device_browser_click_element、device_browser_input_to、device_browser_tap、device_browser_swipe、device_browser_input、device_screen_snapshot、device_screen_analyze

### Changed

- **浏览器工作流**：系统提示更新为 navigate → snapshot → click/fill → wait → repeat 流程

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
