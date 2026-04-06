package shorts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
