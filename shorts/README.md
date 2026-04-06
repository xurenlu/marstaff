# 动漫短剧创作资产（Marstaff）

本目录约定 **纯聊天驱动短剧** 时的落盘结构与中间产物，便于上下文截断后仍可通过 `read_file` 续写。

## 目录约定

复制 [`_template/`](_template/) 为 `shorts/<series_slug>/`，例如：

- `shorts/my_anime/00_series_bible.md` — 系列圣经（世界观、全局 `style`）
- `shorts/my_anime/01_characters.md` — 角色标签句与三层锚点
- `shorts/my_anime/ep01_outline.md` — 单集大纲与节拍
- `shorts/my_anime/ep01_storyboard.md` — 分镜表（含 continuity）

质检使用 [`qc-checklist.md`](qc-checklist.md)。

## 与 `video_story_workflow_create` 的映射

分镜表经确认后，由 Agent 组装为工具参数（见 [`internal/tools/pipeline_executor.go`](../internal/tools/pipeline_executor.go) 中 schema）：

| 分镜表 / 圣经 | 工具参数 |
|----------------|----------|
| 系列标题 + 本集概述 | `name`、`story` |
| `00_series_bible` 全局风格句 | `style` |
| 画幅 / 分辨率 / 帧率 | `aspect_ratio`、`resolution`、`fps` |
| `ep##_storyboard` 每一行 | `scenes[]` 中一项：`prompt`（= 环境 + 机位 + 动作 + 角色标签句 + continuity）、`duration`（秒）、`name` 或 `key`（如 `ep01_sc01`） |

**时长拆分**：若单镜叙事超过单次模型上限（常见 ≤15 秒），拆成多行 scene，用 **continuity** 句衔接；勿将 30 秒压成单个 scene。

**工具选择**：多分镜长视频 **只使用** `video_story_workflow_create`；不要使用 `pipeline_create` 手写拼接流程。

## 相关 Skill

Agent 侧流程模板见 [`skills/anime-short-drama/SKILL.md`](../skills/anime-short-drama/SKILL.md)。
