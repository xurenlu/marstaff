package afk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rocky/marstaff/internal/model"
)

func TestShouldExecuteHeartbeatTaskNeverRun(t *testing.T) {
	s := &Scheduler{}
	task := &model.AFKTask{
		TriggerConfig: model.TriggerConfig{CheckInterval: 5},
		LastExecutionTime: nil,
	}
	require.True(t, s.shouldExecuteHeartbeatTask(task, time.Now()))
}

func TestShouldExecuteHeartbeatTaskIntervalNotElapsed(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	last := now.Add(-2 * time.Minute)
	task := &model.AFKTask{
		TriggerConfig: model.TriggerConfig{CheckInterval: 5},
		LastExecutionTime: &last,
	}
	require.False(t, s.shouldExecuteHeartbeatTask(task, now))
}

func TestShouldExecuteHeartbeatTaskIntervalElapsed(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	last := now.Add(-6 * time.Minute)
	task := &model.AFKTask{
		TriggerConfig: model.TriggerConfig{CheckInterval: 5},
		LastExecutionTime: &last,
	}
	require.True(t, s.shouldExecuteHeartbeatTask(task, now))
}

func TestShouldExecuteHeartbeatTaskDefaultInterval(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	last := now.Add(-31 * time.Minute)
	task := &model.AFKTask{
		TriggerConfig: model.TriggerConfig{CheckInterval: 0},
		LastExecutionTime: &last,
	}
	require.True(t, s.shouldExecuteHeartbeatTask(task, now))
}

func TestShouldPollAsyncTaskFirstPoll(t *testing.T) {
	s := &Scheduler{}
	task := &model.AFKTask{
		LastExecutionTime: nil,
		TriggerConfig: model.TriggerConfig{
			AsyncTaskConfig: &model.AsyncTaskConfig{PollInterval: 30},
		},
	}
	require.True(t, s.shouldPollAsyncTask(task))
}

func TestShouldPollAsyncTaskRespectsInterval(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	last := now.Add(-5 * time.Second)
	task := &model.AFKTask{
		LastExecutionTime: &last,
		TriggerConfig: model.TriggerConfig{
			AsyncTaskConfig: &model.AsyncTaskConfig{PollInterval: 30},
		},
	}
	require.False(t, s.shouldPollAsyncTask(task))
}

func TestShouldPollAsyncTaskDefaultMinPollSeconds(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	last := now.Add(-35 * time.Second)
	task := &model.AFKTask{
		LastExecutionTime: &last,
		TriggerConfig: model.TriggerConfig{
			AsyncTaskConfig: &model.AsyncTaskConfig{PollInterval: 5}, // below 10 uses default 30s floor in code
		},
	}
	require.True(t, s.shouldPollAsyncTask(task))
}

func TestShouldPollAsyncTaskNilConfig(t *testing.T) {
	s := &Scheduler{}
	task := &model.AFKTask{
		LastExecutionTime: nil,
		TriggerConfig:     model.TriggerConfig{AsyncTaskConfig: nil},
	}
	require.False(t, s.shouldPollAsyncTask(task))
}

func TestCalculateNextExecutionAddsOneHour(t *testing.T) {
	s := &Scheduler{}
	from := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	next, err := s.calculateNextExecution("*/5 * * * *", from)
	require.NoError(t, err)
	require.Equal(t, from.Add(time.Hour), next)
}

func TestSchedulerStopIdempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil)
	s.Stop()
	s.Stop() // must not panic
}
