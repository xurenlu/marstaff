# Marstaff

Go 版 AI 助手 - 类似 OpenClaw 的可扩展 AI 助手框架

## 功能特性

- **多 AI 提供商支持**: Z.ai、Qwen (通义千问)，可配置切换
- **多平台 IM 集成**: 网页聊天、Telegram Bot、Matrix 协议
- **WebSocket 实时通信**: 高性能的 WebSocket Gateway
- **技能系统**: 类似 OpenClaw 的 SKILL.md 技能定义
- **会话管理**: 支持树形对话分支
- **持久化记忆**: 用户偏好和上下文持久化

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端语言 | Go 1.21+ |
| 数据库 | MySQL 8.0+ |
| WebSocket | gorilla/websocket |
| ORM | GORM |
| 配置管理 | Viper |
| CLI | Cobra |
| 日志 | zerolog |

## 快速开始

### 前置要求

- Go 1.21 或更高版本
- MySQL 8.0 或更高版本
- (可选) Telegram Bot Token
- (可选) Z.ai / Qwen API Key

### 安装

```bash
# 克隆仓库
git clone https://github.com/rocky/marstaff.git
cd marstaff

# 安装依赖
make deps

# 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的 API 密钥

# 初始化数据库
mysql -u root -p < migrations/001_init_schema.up.sql
```

### 运行

```bash
# 运行 Gateway
make run-gateway

# 运行 Agent
make run-agent
```

## 配置

配置文件位于 `configs/config.yaml`：

```yaml
server:
  host: "0.0.0.0"
  port: 18789

database:
  host: "localhost"
  port: 3306
  database: "marstaff"
  username: "marstaff"
  password: "password"

provider:
  default: "zai"
  zai:
    api_key: "${ZAI_API_KEY}"
    base_url: "https://api.z.ai"
  qwen:
    api_key: "${QWEN_API_KEY}"
    base_url: "https://dashscope.aliyuncs.com"

adapters:
  - type: "telegram"
    enabled: true
    config:
      bot_token: "${TELEGRAM_BOT_TOKEN}"
```

## 技能系统

技能目录结构：

```
skills/
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

## 项目结构

```
marstaff/
├── cmd/              # 应用入口
├── internal/         # 私有代码
│   ├── gateway/      # WebSocket Gateway
│   ├── provider/     # AI 提供商
│   ├── adapter/      # IM 适配器
│   ├── skill/        # 技能系统
│   ├── model/        # 数据模型
│   └── config/       # 配置管理
├── skills/           # 技能目录
├── migrations/       # 数据库迁移
└── configs/          # 配置文件
```

## 开发

```bash
# 运行测试
make test

# 代码格式化
make fmt

# 代码检查
make lint
```

## License

MIT
