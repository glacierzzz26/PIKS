// migrate 应用 migrations/*.sql 到数据库(幂等,按 schema_migrations 跟踪)。
package main

import (
	"context"
	"fmt"
	"os"

	"piks/internal/config"
	"piks/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	n, err := store.ApplyMigrations(ctx, pool, "migrations")
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Printf("migrations applied: %d\n", n)
}
