package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL kosong")
	}

	files := []string{
		"migrations/20260313_add_submit_at.sql",
		"migrations/20260319_add_patrol_attendance_id.sql",
		"migrations/20260320_create_patrol_runs.sql",
		"migrations/20260320_drop_legacy_patrol_scan_unique.sql",
		"migrations/20260320_fix_patrol_route_points_and_zero_runs.sql",
		"migrations/20260321_create_api_error_logs.sql",
		"migrations/20260324_create_app_versions.sql",
		"migrations/20260324_create_app_version_masters.sql",
		"migrations/20260402_create_token_configs.sql",
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		panic(err)
	}

	for _, file := range files {
		fmt.Println("running:", file)

		b, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}

		if _, err := db.Exec(string(b)); err != nil {
			panic(fmt.Errorf("failed %s: %w", file, err))
		}
	}

	fmt.Println("all migrations success")
}
