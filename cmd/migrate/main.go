// Migrate sessions to default user (single-user mode)
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

const defaultUserID = "default"
const platformWeb = "web"

var configFile string

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	rootCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate data to default user (single-user mode)",
		RunE:  run,
	}
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "configs/config.yaml", "config file path")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	ctx := context.Background()
	userRepo := repository.NewUserRepository(db)

	// 1. Get or create default user (use unique email to avoid UNIQUE constraint on empty string)
	defaultUser, err := userRepo.GetByPlatformID(ctx, platformWeb, defaultUserID)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		defaultUser = &model.User{
			Platform:       platformWeb,
			PlatformUserID: defaultUserID,
			Username:       defaultUserID,
			Email:          "default@marstaff.local", // unique, avoids conflict with empty email
		}
		if err := userRepo.Create(ctx, defaultUser); err != nil {
			return fmt.Errorf("create default user: %w", err)
		}
		log.Info().Str("user_id", defaultUser.ID).Msg("created default user")
	} else if err != nil {
		return fmt.Errorf("get default user: %w", err)
	} else {
		log.Info().Str("user_id", defaultUser.ID).Msg("default user ready")
	}

	// 2. Find all web users except default
	var otherUsers []model.User
	if err := db.WithContext(ctx).
		Where("platform = ? AND platform_user_id != ?", platformWeb, defaultUserID).
		Find(&otherUsers).Error; err != nil {
		return fmt.Errorf("list other users: %w", err)
	}

	if len(otherUsers) == 0 {
		log.Info().Msg("no other users to migrate, done")
		return nil
	}

	log.Info().Int("count", len(otherUsers)).Msg("migrating sessions from other users")

	otherUserIDs := make([]string, len(otherUsers))
	for i, u := range otherUsers {
		otherUserIDs[i] = u.ID
	}

	// 3. Migrate sessions
	var sessionsUpdated int64
	result := db.WithContext(ctx).
		Model(&model.Session{}).
		Where("user_id IN ?", otherUserIDs).
		Update("user_id", defaultUser.ID)
	if result.Error != nil {
		return fmt.Errorf("update sessions: %w", result.Error)
	}
	sessionsUpdated = result.RowsAffected

	// 4. Migrate memories
	memResult := db.WithContext(ctx).
		Model(&model.Memory{}).
		Where("user_id IN ?", otherUserIDs).
		Update("user_id", defaultUser.ID)
	memoriesUpdated := int64(0)
	if memResult.Error == nil {
		memoriesUpdated = memResult.RowsAffected
	}

	log.Info().
		Int("users_migrated", len(otherUsers)).
		Int64("sessions_migrated", sessionsUpdated).
		Int64("memories_migrated", memoriesUpdated).
		Msg("migration completed")

	return nil
}
