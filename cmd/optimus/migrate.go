package main

import (
	"database/sql"
	"flag"
	"fmt"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ArkGravity/optimus/internal/infra/config"
	"github.com/ArkGravity/optimus/internal/infra/log"
	"github.com/ArkGravity/optimus/migrations"
)

// runMigrate applies the Goose SQL migrations embedded into the binary against
// the configured Postgres instance. It exits 0 on success (including when the
// database is already at head) and 1 on any failure.
func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath, "path to config")
	direction := fs.String("dir", "up", "up | down | status")
	_ = fs.Parse(args)

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		fail("resolve config path", err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		fail("load config", err)
	}
	if err := cfg.ValidateForMigrate(); err != nil {
		fail("validate config", err)
	}

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus migrate starting", "direction", *direction)

	db, err := sql.Open("pgx", cfg.Database.DSN)
	if err != nil {
		fail("open db", err)
	}
	defer db.Close()

	if err := runGoose(db, *direction); err != nil {
		fail("migrate "+*direction, err)
	}
	logger.Info("optimus migrate done")
}

func runGoose(db *sql.DB, direction string) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch direction {
	case "up":
		return goose.Up(db, ".")
	case "down":
		return goose.Down(db, ".")
	case "status":
		return goose.Status(db, ".")
	default:
		return fmt.Errorf("unknown direction: %s", direction)
	}
}
