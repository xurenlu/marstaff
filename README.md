# Marstaff

[![Version](https://img.shields.io/badge/version-1.20.0--rc2-blue.svg)](CHANGELOG.md)

Go 版 AI Agent 平台 —— 可扩展的智能助手框架，支持多模态对话、工具调用、设备控制与离场任务。

## 功能特性

### 核心能力

- **多 AI 提供商**：Z.ai、Qwen（通义千问）、Gemini、DeepSeek、OpenAI、Zhipu、Minimax，可配置切换
- **多模态对话**：支持图片识别（Vision），可发截图让 AI 分析界面、代码、文档
- **多平台接入**：Web 聊天、Telegram Bot、Matrix 协议
- **WebSocket 实时通信**：流式输出、思考过程展示、打字状态
- **技能系统**：SKILL.md 格式，类似 OpenClaw 的可扩展技能定义
- **会话管理**：对话摘要、持久化记忆、项目分组
- **规则与 MCP**：自定义规则、Model Context Protocol 集成

### 工具生态

| 类别 | 工具 |
|------|------|
| 文件与命令 | `read_file`、`write_file`、`list_dir`、`search_files`、`run_command`（安全校验） |
| 设备控制 | Windows（robotgo）、Android（ADB）、Browser（chromedp）自动化 |
| 媒体生成 | 文生图、文生视频（万像 2.6 等） |
| 工作流 | Git 操作、浏览器搜索、定时任务（cron）、TODO 待办 |
| 安装管理 | 技能、规则、MCP 服务器的安装与配置 |
| AFK 任务 | 定时任务、AI 分析任务、事件触发（股票、API、文件监控） |

### AFK 离场模式

支持创建后台监控任务，在用户离开时自动执行：

- **定时任务**：Cron 表达式触发
- **AI 驱动**：定期调用 AI 分析（如日志、新闻）
- **事件驱动**：股票价格、API 检查、文件变更、日志匹配
- **通知**：Telegram、Resend 邮件、WebSocket 推送

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go 1.25+ |
| Web 框架 | Gin |
| 数据库 | MySQL 8.0+，GORM |
| WebSocket | gorilla/websocket |
| 浏览器自动化 | chromedp |
| 桌面自动化 | robotgo（Windows） |
| 配置 | Viper + 环境变量 |
| CLI | Cobra |
| 日志 | zerolog |

## 快速开始

### 前置要求

- Go 1.25+
- MySQL 8.0+
- （可选）AI 提供商 API Key：Z.ai / Qwen / Gemini 等
- （可选）阿里云 OSS：用于截图、生成图片/视频存储
- （可选）Telegram Bot Token：AFK 任务通知

### 安装

```bash
git clone https://github.com/rocky/marstaff.git
cd marstaff

# 安装依赖
make deps

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入 API 密钥

# 初始化数据库
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS marstaff CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mysql -u root -p marstaff < migrations/001_init_schema.up.sql
# 依次执行其他 migrations 中的 .up.sql
```

### 运行

```bash
# 构建并启动（单进程，包含 Web 与 Agent）
make run

# 或指定配置文件
./bin/gateway --config configs/config.yaml
```

默认访问：http://localhost:18789

### Docker

```bash
make docker-build
make docker-run
```

## 配置

主配置文件：`configs/config.yaml`。敏感信息通过环境变量注入，如 `${ZHIPU_API_KEY}`。

### 环境变量示例

```bash
# AI 提供商
ZHIPU_API_KEY=xxx
QWEN_API_KEY=xxx
GEMINI_API_KEY=xxx

# 阿里云 OSS（截图、媒体存储）
ALIYUN_ACCESS_KEY_ID=xxx
ALIYUN_ACCESS_KEY_SECRET=xxx

# 可选
TELEGRAM_BOT_TOKEN=xxx
RESEND_API_KEY=xxx
RESEND_FROM_EMAIL=xxx
```

### Web 界面语言

文案位于 `web/static/locales/en.json` 与 `zh.json`，由 `web/static/js/marstaff-i18n.js` 加载。浏览器可设置 `localStorage.setItem('marstaff_locale', 'zh')` 或 `'en'` 后刷新页面。

### 安全配置

文件与命令工具通过 `configs/security.yaml` 控制路径白名单与命令白名单，详见该文件。

## 项目结构

```
marstaff/
├── cmd/
│   ├── gateway/      # 主服务入口（Web + Agent + WebSocket）
│   └── migrate/      # 数据迁移（单用户模式）
├── internal/
│   ├── agent/        # Agent 引擎、工具执行、摘要、记忆
│   ├── api/          # REST API
│   ├── gateway/      # WebSocket Hub、OSS 上传
│   ├── provider/     # AI 提供商（Z.ai、Qwen、Gemini 等）
│   ├── media/        # 媒体生成（图片、视频）
│   ├── device/       # 设备控制（Windows、Android、Browser）
│   ├── tools/        # 文件、命令、Git、AFK、安装等工具
│   ├── afk/          # AFK 任务调度与通知
│   ├── skill/        # 技能系统
│   ├── model/        # 数据模型
│   └── config/       # 配置管理
├── skills/           # 技能目录
├── web/              # 前端模板与静态资源
├── migrations/       # 数据库迁移
└── configs/          # 配置文件
```

## 技能系统

技能目录结构：

```
skills/
├── calculator/
│   └── SKILL.md
└── weather/
    └── SKILL.md
```

SKILL.md 格式：

```markdown
---
id: weather
name: Weather
description: Get weather information
version: 1.0.0
author: Your Name
category: utilities
tags: [weather, forecast]
---

# Weather Skill

This skill provides weather information for any location.

## Usage

Ask "What's the weather in Beijing?" or "天气怎么样？"
```

## 常用命令

```bash
make build          # 构建
make run            # 构建并运行
make test           # 运行测试
make fmt            # 格式化代码
make lint           # 代码检查
make migrate-single-user  # 迁移到单用户模式
```

## 开发

```bash
make test
make fmt
make lint
```

## 版本与变更

- **当前版本**：`1.20.0-rc2`
- **变更记录**：[CHANGELOG.md](CHANGELOG.md)
- **API 版本**：响应头 `X-Marstaff-Version`、`/api/health` 返回 `version` 字段

## License

MIT
