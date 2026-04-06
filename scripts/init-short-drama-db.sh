#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATE_DIR="$REPO_ROOT/shorts/_template"
SCHEMA_SQL="$TEMPLATE_DIR/sql/schema_v1.sql"

usage() {
  cat <<USAGE
Usage: $0 <series_slug> [title] [style]

Creates a short drama project directory and initializes the SQLite sidecar DB.

  series_slug  Directory name under shorts/ (e.g. my_anime)
  title        Series title (default: same as slug)
  style        Global style snapshot (optional)

Example:
  $0 my_anime "我的校园短剧" "Japanese TV anime cel shading, clean lineart"
USAGE
  exit 1
}

if [ $# -lt 1 ]; then
  usage
fi

SLUG="$1"
TITLE="${2:-$SLUG}"
STYLE="${3:-}"

if [[ "$SLUG" =~ [[:space:]] ]]; then
  echo "Error: series_slug must not contain spaces" >&2
  exit 1
fi

SERIES_DIR="$REPO_ROOT/shorts/$SLUG"

if [ -f "$SERIES_DIR/drama.sqlite" ]; then
  echo "Error: $SERIES_DIR/drama.sqlite already exists" >&2
  exit 1
fi

if ! command -v sqlite3 &>/dev/null; then
  echo "Error: sqlite3 is not installed" >&2
  exit 1
fi

mkdir -p "$SERIES_DIR"

# Copy Markdown templates
for tpl in 00_series_bible.md 01_characters.md ep01_outline.md ep01_storyboard.md; do
  if [ -f "$TEMPLATE_DIR/$tpl" ] && [ ! -f "$SERIES_DIR/$tpl" ]; then
    cp "$TEMPLATE_DIR/$tpl" "$SERIES_DIR/$tpl"
  fi
done

# Copy schema_version
cp "$TEMPLATE_DIR/schema_version.txt" "$SERIES_DIR/schema_version"

# Create and initialize SQLite DB
sqlite3 "$SERIES_DIR/drama.sqlite" < "$SCHEMA_SQL"

# Insert series row
ESCAPED_TITLE=$(echo "$TITLE" | sed "s/'/''/g")
ESCAPED_STYLE=$(echo "$STYLE" | sed "s/'/''/g")
sqlite3 "$SERIES_DIR/drama.sqlite" \
  "INSERT INTO series (slug, title, style_snapshot) VALUES ('$SLUG', '$ESCAPED_TITLE', '$ESCAPED_STYLE');"

echo "Short drama project initialized:"
echo "  Directory: shorts/$SLUG/"
echo "  Database:  shorts/$SLUG/drama.sqlite"
echo "  Schema:    v$(sqlite3 "$SERIES_DIR/drama.sqlite" 'PRAGMA user_version;')"
echo ""
echo "Next steps:"
echo "  1. Edit shorts/$SLUG/00_series_bible.md"
echo "  2. Edit shorts/$SLUG/01_characters.md"
echo "  3. In chat, set Session.Metadata:"
echo "     {\"short_drama\":{\"series_slug\":\"$SLUG\",\"db_relative_path\":\"shorts/$SLUG/drama.sqlite\",\"schema_user_version\":1}}"
