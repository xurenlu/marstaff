package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCodingStatsAutoMigrateParsesDefaultTags(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:coding_stats_test?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&Project{}, &CodingStats{})
	require.NoError(t, err)
}
