---
name: anime-short-drama
description: 在 Marstaff 聊天中从零完成动漫短剧——系列圣经、角色标签句、分镜与 continuity、侧车 SQLite 资源库（抗摘要）、调用 video_story_workflow_create 落地多分镜成片与质检返工。当用户要做动漫短剧、条漫视频、分镜脚本、或解决 AI 角色脸盲/一致性时使用。
version: 1.1.0
---

# 动漫短剧（纯聊天全流程）

## 何时触发

- 用户要做 **动漫/番剧风短剧**、**多分镜视频**、**按集创作**。
- 用户抱怨 **角色分不清、脸长得一样**。
- 用户提到 **分镜、拼接、每集、角色设定、风格统一**。
- 用户担心 **对话摘要压缩后找不到素材**、需要 **本地资源索引**。

## 核心契约（Agent 必须遵守）

1. **先文档后工具**：在用户确认 [`shorts/<series>/01_characters.md`](../../shorts/_template/01_characters.md)（或会话内等价摘要）前，不调用 `video_story_workflow_create`。
2. **标签句强制复用**：每个含角色的 scene，`prompt` 必须嵌入对应 **完整角色标签句**（见模板）。
3. **单集冻结**：本集锁定后改设定须 **bump 版本号**，避免混用。
4. **工具选择**：长故事多分镜 **只用** `video_story_workflow_create`；**禁止**用 `pipeline_create` 手搓 ffmpeg/拼接；与 [`internal/agent/engine.go`](../../internal/agent/engine.go) 系统提示一致。
5. **每 scene 时长**：落在单次视频模型上限内（常见 **≤15 秒**）；超长镜头拆成多 scene，用 **continuity** 衔接。

## 侧车 SQLite 资源库（抗摘要压缩）

Marstaff 主库（MySQL）存 [`Pipeline`](../../internal/model/pipeline.go) 等运行时数据；**每剧一个本地 SQLite** 作为创作侧索引：人物、分镜行、生成视频 URL、可选 `pipeline_id`。**对话 `Summary` 会变，不要把唯一真相只放在摘要里**；稳定指针放在 **会话 Metadata**（见下）。

### 路径（写死）

| 路径 | 说明 |
|------|------|
| `shorts/<series_slug>/drama.sqlite` | 唯一资源库文件 |
| `shorts/<series_slug>/schema_version` | 纯文本，与 `PRAGMA user_version` 一致（复制自模板 [`schema_version.txt`](../../shorts/_template/schema_version.txt)） |

初始化 SQL：[`shorts/_template/sql/schema_v1.sql`](../../shorts/_template/sql/schema_v1.sql)。步骤见 [`shorts/_template/sql/README.md`](../../shorts/_template/sql/README.md)。

### Session.Metadata JSON（必须写入稳定指针）

建剧或首次落库后，应写入（或提示用户写入）会话 **Metadata**，例如：

```json
{
  "short_drama": {
    "series_slug": "my_anime",
    "db_relative_path": "shorts/my_anime/drama.sqlite",
    "schema_user_version": 1
  }
}
```

- **`db_relative_path`**：相对**仓库根**的路径，便于摘要后仍可用 `read_file` / 终端 `sqlite3` 打开。
- 摘要压缩后：只要 Metadata 未丢，即可 **定位库文件** 并 `SELECT` 分镜与素材 URL。

### 与 Markdown 的双写策略（推荐 A）

- **A（推荐）**：`00_series_bible.md`、`01_characters.md`、`ep##_storyboard.md` 仍为 **人类可读主源**；定稿或每轮生成后，Agent **把角色行与分镜行同步进 SQLite**（`characters`、`scene`），避免「只在一处更新」。
- **B**：以 SQLite 为主、Markdown 由导出生成（工作量大，默认不采用）。

### 视频与流水线写入 `asset`

工作流返回分镜 URL 或最终成片 URL 后，插入 `asset` 表（视频在 OSS，**只存 URL**）：

- `kind`：`scene_video` | `final_concat` | `ref_image` | `audio` | `other`
- `url`：完整 HTTPS 地址
- `pipeline_id`：Marstaff 工作流 ID（与 MySQL 一致，可空）
- `scene_key`：如 `ep01_sc03`（可空，成片可为空）
- `attempt`：同镜多版本时递增；`is_selected`：0/1 标记当前采用条

可选：`pipeline_ref` 表登记「本集主渲染」等说明，便于从聊天跳转到 `/workflows`。

### 查询与并发

- 优先只读：`sqlite3 shorts/<slug>/drama.sqlite "SELECT ..."`。
- 单用户单剧单文件；多进程同时写时启用 WAL 并约定 **单写者**（通常仅 Agent 顺序写入）。

## 会话内产出物（落盘路径）

若 `write_file` 可用，在仓库中创建：

| 文件 | 用途 |
|------|------|
| `shorts/<slug>/00_series_bible.md` | 世界观、全局 `style`、视觉母题 |
| `shorts/<slug>/01_characters.md` | 角色标签句与三层锚点 |
| `shorts/<slug>/ep##_outline.md` | 节拍表 |
| `shorts/<slug>/ep##_storyboard.md` | 分镜表（含 continuity） |
| `shorts/<slug>/drama.sqlite` | 侧车库（gitignore，勿提交） |

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
- **双写不一致**：改分镜 Markdown 后必须同步 SQLite（或约定仅在一处改并导出）。

## 相关仓库路径

- 模板：[`shorts/_template/`](../../shorts/_template/)
- 说明：[`shorts/README.md`](../../shorts/README.md)
- Schema v1：[`shorts/_template/sql/schema_v1.sql`](../../shorts/_template/sql/schema_v1.sql)
