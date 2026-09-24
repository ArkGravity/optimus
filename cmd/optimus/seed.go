package main

import (
	"context"
	"flag"
	"path/filepath"

	"github.com/ArkGravity/optimus/internal/infra/config"
	"github.com/ArkGravity/optimus/internal/infra/db"
	"github.com/ArkGravity/optimus/internal/infra/log"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/seed"
)

// runSeed registers permission codes and creates the builtin RBAC graph. The
// initial administrator password is printed exactly once.
func runSeed(args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath, "path to config")
	_ = fs.Parse(args)

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		fail("resolve config path", err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		fail("load config", err)
	}
	if err := cfg.ValidateStrict(); err != nil {
		fail("validate config", err)
	}
	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})

	gdb, err := db.Open(cfg.Database)
	if err != nil {
		fail("open db", err)
	}

	if r, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		fail("register permissions", err)
	} else {
		logger.Info("permissions registered", "inserted", r.Inserted, "updated", r.Updated, "stale", r.Stale)
	}

	res, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: cfg.Boot.AdminUsername,
		AdminEmail:    cfg.Boot.AdminEmail,
		BcryptCost:    cfg.Auth.BcryptCost,
	})
	if err != nil {
		fail("seed", err)
	}

	if res.AdminInitialPassword != "" {
		logger.Warn(
			"INITIAL ADMIN CREDENTIALS — RECORD THESE NOW (printed only once)",
			"username", cfg.Boot.AdminUsername,
			"password", res.AdminInitialPassword,
		)
	} else {
		logger.Info("admin user already exists; no password generated")
	}
}
