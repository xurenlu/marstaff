# Marstaff Skills

本目录存放 AI Agent 可用的技能（skills）。每个技能是一个子目录，内含 `SKILL.md` 及可选参考文件。

## 来源

- **内置技能**：`calculator`、`weather` 等由 Marstaff 项目维护
- **社区技能**：来自 [openclaw-master-skills](https://github.com/LeoYeAI/openclaw-master-skills)，127+ 精选 OpenClaw 技能
- **Agent Reach**：`agent-reach` 来自 [Panniantong/Agent-Reach](https://github.com/Panniantong/Agent-Reach)，为 Agent 提供互联网能力（Twitter、Reddit、YouTube、B站、小红书等）

## 更新社区技能

```bash
make skills-update
```

会从 openclaw-master-skills 仓库拉取最新技能并合并到本目录，内置技能不会被覆盖。
