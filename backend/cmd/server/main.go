// Command server is the RefratIA HTTP backend: it opens and migrates the
// database, then serves the routes composed in src/app.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"refratia/backend/src/app"
	"refratia/backend/src/shared/database"
)

func main() {
	databaseContext, cancelDatabaseContext := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancelDatabaseContext()

	appDatabase, databaseOpenError := database.Open(databaseContext)
	if databaseOpenError != nil {
		panic(databaseOpenError)
	}
	if databaseMigrationError := appDatabase.Migrate(); databaseMigrationError != nil {
		panic(fmt.Errorf("migrate postgres schema: %w", databaseMigrationError))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	address := ":" + port
	fmt.Printf("Backend running on http://localhost%s\n", address)
	if serveError := http.ListenAndServe(address, app.Routes(appDatabase.Gorm)); serveError != nil {
		panic(serveError)
	}
}
