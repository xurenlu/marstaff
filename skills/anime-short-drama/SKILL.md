---
name: anime-short-drama
description: 在 Marstaff 聊天中从零完成动漫短剧——系列圣经、角色标签句、分镜与 continuity、调用 video_story_workflow_create 落地多分镜成片与质检返工。当用户要做动漫短剧、条漫视频、分镜脚本、或解决 AI 角色脸盲/一致性时使用。
version: 1.0.0
---

# 动漫短剧（纯聊天全流程）

## 何时触发

- 用户要做 **动漫/番剧风短剧**、**多分镜视频**、**按集创作**。
- 用户抱怨 **角色分不清、脸长得一样**。
- 用户提到 **分镜、拼接、每集、角色设定、风格统一**。

## 核心契约（Agent 必须遵守）

1. **先文档后工具**：在用户确认 [`shorts/<series>/01_characters.md`](../../shorts/_template/01_characters.md)（或会话内等价摘要）前，不调用 `video_story_workflow_create`。
2. **标签句强制复用**：每个含角色的 scene，`prompt` 必须嵌入对应 **完整角色标签句**（见模板）。
3. **单集冻结**：本集锁定后改设定须 **bump 版本号**，避免混用。
4. **工具选择**：长故事多分镜 **只用** `video_story_workflow_create`；**禁止**用 `pipeline_create` 手搓 ffmpeg/拼接；与 [`internal/agent/engine.go`](../../internal/agent/engine.go) 系统提示一致。
5. **每 scene 时长**：落在单次视频模型上限内（常见 **≤15 秒**）；超长镜头拆成多 scene，用 **continuity** 衔接。

## 会话内产出物（落盘路径）

若 `write_file` 可用，在仓库中创建：

| 文件 | 用途 |
|------|------|
| `shorts/<slug>/00_series_bible.md` | 世界观、全局 `style`、视觉母题 |
| `shorts/<slug>/01_characters.md` | 角色标签句与三层锚点 |
| `shorts/<slug>/ep##_outline.md` | 节拍表 |
| `shorts/<slug>/ep##_storyboard.md` | 分镜表（含 continuity） |

模板来源：[`shorts/_template/`](../../shorts/_template/)。若用户拒绝落盘，每条长回复末尾附 **同结构设定摘要块**（可复制）。

## 角色可区分性（解决脸盲）

每张脸至少锁 **三层**：剪影/发型、固定配色、标志道具或伤痕；再可选 **站位习惯**（A 左 B 右）。  
为每人写一行 **50–120 字标签句**，之后 **每镜必贴**。控制同框人数：**每集核心角色建议 ≤3**。

## 分镜表 → `video_story_workflow_create`

工具 schema 见 [`internal/tools/pipeline_executor.go`](../../internal/tools/pipeline_executor.go)（`video_story_workflow_create`）。

**参数映射**

- `name`：工作流显示名，建议 `series_ep01` 形式。
- `story`：系列 + 本集概述（人类可读）。
- `style`：从系列圣经复制的 **全局风格句**（与各 scene 不冲突）。
- `aspect_ratio` / `resolution` / `fps`：与圣经、平台一致。
- `scenes`：数组；每项至少 `prompt`，可选 `duration`、`name`、`key`。

**单条 `prompt` 推荐拼装顺序**

1. 环境一句  
2. 景别 + 机位  
3. 动作与镜头运动  
4. 在场角色 **完整标签句**  
5. **continuity**（衔接上一镜：站位、光线、服装）

**`name` / `key`**：建议 `ep01_sc01` 便于检索与重跑。

**示例（JSON 片段，仅作结构参考；实际由工具调用传入）**

```json
{
  "name": "demo_series_ep01",
  "story": "第一集：主角在雨夜车站遇见神秘转学生。",
  "style": "Japanese TV anime cel shading, clean lineart, flat colors, low noise",
  "aspect_ratio": "16:9",
  "scenes": [
    {
      "key": "ep01_sc01",
      "name": "establishing",
      "duration": 10,
      "prompt": "Rainy night train station platform, wide shot, wet reflections; CHAR_A: ... (full tag line); opening scene establishing mood"
    },
    {
      "key": "ep01_sc02",
      "name": "meet",
      "duration": 12,
      "prompt": "Same station, medium shot; CHAR_A and CHAR_B: ... (both tag lines); continuity: CHAR_A still on left side of frame, same rain intensity"
    }
  ]
}
```

## 可选前置：风格板与立绘

在烧视频前，若存在 `generate_image`：先生成 **无剧情** 风格板 1–3 张，再按需 **角色立绘**；视频 prompt 仍复用同一套标签句。纯文本 **无法保证** 像素级同脸，目标定为 **可区分 + 风格统一**。

## 质检与返工

使用 [`shorts/qc-checklist.md`](../../shorts/qc-checklist.md)：区分度、连贯性、节奏与可生成性。不通过时按清单 **返工顺序** 修改文档后再建工作流。

## 风险与预期

- 无参考图时，跨大量镜头 **同一面部** 可能漂移；用 **强标签 + 减少双人大特写** 缓解。
- 版权：避免可识别第三方 IP、商标与真人肖像。

## 相关仓库路径

- 模板：[`shorts/_template/`](../../shorts/_template/)
- 说明：[`shorts/README.md`](../../shorts/README.md)
