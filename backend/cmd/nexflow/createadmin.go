package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runCreateAdmin implements `nexflow create-admin USERNAME "Full name"`.
// The password is read from the NEXFLOW_ADMIN_PASSWORD variable, or from a
// line on standard input when the variable is not set. Works only on an empty database.
func runCreateAdmin(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf(`usage: nexflow create-admin USERNAME "Full name"`)
	}
	dbURL := os.Getenv("NEXFLOW_DB_URL")
	if dbURL == "" {
		return fmt.Errorf("NEXFLOW_DB_URL is not set")
	}
	password := os.Getenv("NEXFLOW_ADMIN_PASSWORD")
	if password == "" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("no password given: set NEXFLOW_ADMIN_PASSWORD or pipe one line on stdin")
		}
		password = strings.TrimRight(line, "\r\n")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrateUp(pool); err != nil {
		return err
	}
	u, err := access.CreateFirstAdmin(ctx, pool, args[0], args[1], password)
	if err != nil {
		return err
	}
	fmt.Printf("created admin %s\n", u.Username)
	return nil
}
