package shorts

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from test working directory")
		}
		dir = parent
	}
}

func TestTemplateFilesExist(t *testing.T) {
	root := findRepoRoot(t)
	required := []string{
		"shorts/_template/00_series_bible.md",
		"shorts/_template/01_characters.md",
		"shorts/_template/ep01_outline.md",
		"shorts/_template/ep01_storyboard.md",
		"shorts/_template/schema_version.txt",
		"shorts/_template/sql/schema_v1.sql",
		"shorts/_template/sql/README.md",
		"shorts/qc-checklist.md",
		"shorts/README.md",
		"skills/anime-short-drama/SKILL.md",
	}
	for _, rel := range required {
		path := filepath.Join(root, rel)
		_, err := os.Stat(path)
		require.NoError(t, err, "expected file %s", path)
	}
}

func TestSchemaV1SetsUserVersion(t *testing.T) {
	root := findRepoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "shorts/_template/sql/schema_v1.sql"))
	require.NoError(t, err)
	body := string(b)
	require.Contains(t, body, "PRAGMA user_version = 1")
	require.True(t, strings.Contains(body, "CREATE TABLE IF NOT EXISTS series"))
	require.True(t, strings.Contains(body, "CREATE TABLE IF NOT EXISTS asset"))
}

func TestSchemaV1ExecuteAndCRUD(t *testing.T) {
	root := findRepoRoot(t)
	schemaSQL, err := os.ReadFile(filepath.Join(root, "shorts/_template/sql/schema_v1.sql"))
	require.NoError(t, err)

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(string(schemaSQL))
	require.NoError(t, err, "schema_v1.sql must execute without errors")

	var userVersion int
	err = db.QueryRow("PRAGMA user_version").Scan(&userVersion)
	require.NoError(t, err)
	require.Equal(t, 1, userVersion)

	// Insert series
	res, err := db.Exec(`INSERT INTO series (slug, title, style_snapshot) VALUES ('test_drama', 'Test Drama', 'cel shading')`)
	require.NoError(t, err)
	seriesID, _ := res.LastInsertId()

	// Insert character
	_, err = db.Exec(`INSERT INTO characters (series_id, char_id, tag_line) VALUES (?, 'CHAR_A', 'teen girl, pink twin-tails, pink dress')`, seriesID)
	require.NoError(t, err)

	// Insert episode
	res, err = db.Exec(`INSERT INTO episode (series_id, ep_index, title) VALUES (?, 1, 'Episode 1')`, seriesID)
	require.NoError(t, err)
	epID, _ := res.LastInsertId()

	// Insert scene
	_, err = db.Exec(`INSERT INTO scene (episode_id, scene_key, prompt, duration_sec, sort_order, continuity) VALUES (?, 'ep01_sc01', 'rainy station', 10, 1, 'opening')`, epID)
	require.NoError(t, err)

	// Insert asset
	_, err = db.Exec(`INSERT INTO asset (kind, url, pipeline_id, scene_key, episode_id, attempt) VALUES ('scene_video', 'https://oss.example.com/v1.mp4', 42, 'ep01_sc01', ?, 1)`, epID)
	require.NoError(t, err)

	// Query: scenes for episode
	rows, err := db.Query(`SELECT scene_key, prompt, duration_sec FROM scene WHERE episode_id = ? ORDER BY sort_order`, epID)
	require.NoError(t, err)
	defer rows.Close()

	var sceneCount int
	for rows.Next() {
		var key, prompt string
		var dur int
		require.NoError(t, rows.Scan(&key, &prompt, &dur))
		require.Equal(t, "ep01_sc01", key)
		require.Equal(t, 10, dur)
		sceneCount++
	}
	require.Equal(t, 1, sceneCount)

	// Query: selected assets
	var assetURL string
	err = db.QueryRow(`SELECT url FROM asset WHERE scene_key = 'ep01_sc01' AND kind = 'scene_video' ORDER BY attempt DESC LIMIT 1`).Scan(&assetURL)
	require.NoError(t, err)
	require.Equal(t, "https://oss.example.com/v1.mp4", assetURL)

	// Uniqueness: duplicate scene_key within same episode should fail
	_, err = db.Exec(`INSERT INTO scene (episode_id, scene_key, prompt, sort_order) VALUES (?, 'ep01_sc01', 'dup', 2)`, epID)
	require.Error(t, err, "duplicate scene_key in same episode should violate UNIQUE constraint")

	// FK: delete episode cascades scene
	_, err = db.Exec(`DELETE FROM episode WHERE id = ?`, epID)
	require.NoError(t, err)
	var sceneAfterDelete int
	err = db.QueryRow(`SELECT COUNT(*) FROM scene WHERE episode_id = ?`, epID).Scan(&sceneAfterDelete)
	require.NoError(t, err)
	require.Equal(t, 0, sceneAfterDelete)

	// asset.kind CHECK constraint
	_, err = db.Exec(`INSERT INTO asset (kind, url) VALUES ('INVALID_KIND', 'https://x.com/bad.mp4')`)
	require.Error(t, err, "invalid kind should violate CHECK constraint")
}
