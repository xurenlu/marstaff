# 短剧 SQLite 初始化（v1）

在仓库根目录执行（将 `my_series` 换成你的 `series_slug`）：

```bash
mkdir -p shorts/my_series
sqlite3 shorts/my_series/drama.sqlite < shorts/_template/sql/schema_v1.sql
cp shorts/_template/schema_version.txt shorts/my_series/schema_version
```

校验：

```bash
sqlite3 shorts/my_series/drama.sqlite "PRAGMA user_version;"
sqlite3 shorts/my_series/drama.sqlite ".schema"
```

插入 `series` 行示例：

```sql
INSERT INTO series (slug, title, style_snapshot) VALUES ('my_series', '我的短剧', 'Japanese TV anime cel shading, clean lineart');
```
