# afk_create_oneoff_task 装配说明

`afk_create_oneoff_task` 工具需要主程序完成装配后才能使用。在创建 `AFKExecutor` 并调用 `RegisterBuiltInTools()` 之后，调用：

```go
afkExecutor.SetupOneOffTasks(sessionRepo, asyncNotifier, validator)
```

参数说明：
- `sessionRepo`: `*repository.SessionRepository`，用于解析 session work_dir 和更新 AFK 状态
- `asyncNotifier`: `afk.AsyncTaskNotifier`，用于 WebSocket 实时通知（可与 Scheduler 的 TaskExecutor 共用同一实例）
- `validator`: `tools.CommandValidator`，可选，用于校验命令安全性（如 `toolsExecutor.GetValidator()`）

若未装配，调用 `afk_create_oneoff_task` 将返回错误："one-off tasks not configured"。
