-- Marstaff 短剧侧车库 v1（与 PRAGMA user_version 一致）
-- 用法（仓库根目录）：sqlite3 shorts/<series_slug>/drama.sqlite < shorts/_template/sql/schema_v1.sql

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS series (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  style_snapshot TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS characters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  char_id TEXT NOT NULL,
  tag_line TEXT NOT NULL,
  notes TEXT,
  UNIQUE (series_id, char_id)
);

CREATE TABLE IF NOT EXISTS episode (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  ep_index INTEGER NOT NULL,
  title TEXT,
  UNIQUE (series_id, ep_index)
);

CREATE TABLE IF NOT EXISTS scene (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  episode_id INTEGER NOT NULL REFERENCES episode(id) ON DELETE CASCADE,
  scene_key TEXT NOT NULL,
  prompt TEXT NOT NULL,
  duration_sec INTEGER,
  sort_order INTEGER NOT NULL DEFAULT 0,
  continuity TEXT,
  UNIQUE (episode_id, scene_key)
);

CREATE TABLE IF NOT EXISTS asset (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL CHECK (
    kind IN (
      'scene_video',
      'final_concat',
      'ref_image',
      'audio',
      'other'
    )
  ),
  url TEXT NOT NULL,
  pipeline_id INTEGER,
  scene_key TEXT,
  episode_id INTEGER REFERENCES episode(id) ON DELETE SET NULL,
  attempt INTEGER NOT NULL DEFAULT 1,
  is_selected INTEGER NOT NULL DEFAULT 0 CHECK (is_selected IN (0, 1)),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  notes TEXT
);

CREATE TABLE IF NOT EXISTS pipeline_ref (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  pipeline_id INTEGER NOT NULL,
  role TEXT,
  note TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_scene_episode_sort ON scene(episode_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_scene_key ON scene(scene_key);
CREATE INDEX IF NOT EXISTS idx_asset_pipeline ON asset(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_asset_kind_selected ON asset(kind, is_selected);
CREATE INDEX IF NOT EXISTS idx_characters_series ON characters(series_id);
CREATE INDEX IF NOT EXISTS idx_episode_series ON episode(series_id);

PRAGMA user_version = 1;
