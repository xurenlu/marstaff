package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rocky/marstaff/internal/model"
)

func TestMigrationModelsIncludePipelineStep(t *testing.T) {
	models := migrationModels()

	hasPipeline := false
	hasPipelineStep := false
	for _, candidate := range models {
		switch candidate.(type) {
		case *model.Pipeline:
			hasPipeline = true
		case *model.PipelineStep:
			hasPipelineStep = true
		}
	}

	require.True(t, hasPipeline, "pipeline model should be migrated")
	require.True(t, hasPipelineStep, "pipeline step model should be migrated")
}
