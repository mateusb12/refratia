// Command reconcile is REPORT-ONLY (see package reconcile): it deletes nothing.
// Run against the environment you load; with REFRATIA_NO_DOTENV=1 it never
// reads the developer .env.
package main

import (
	"context"
	"fmt"
	"os"

	"refratia/backend/src/features/persistence/reconcile"
	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
)

func main() {
	ctx := context.Background()
	db, err := database.Open(ctx)
	check(err)
	defer db.Close()
	client, err := fileStorage.New(ctx)
	check(err)
	findings, err := reconcile.Run(ctx, db.Gorm, client, os.Getenv("BUCKET_NAME"))
	check(err)
	for _, f := range findings {
		fmt.Printf("%s\t%s\n", f.Kind, f.Detail)
	}
	fmt.Printf("%d finding(s)\n", len(findings))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "reconcile:", err)
		os.Exit(1)
	}
}
