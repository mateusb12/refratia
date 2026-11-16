// Command server is the RefratIA HTTP backend: it opens and migrates the
// database, then serves the routes composed in src/app.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"refratia/backend/src/app"
	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/ocr"
	"refratia/backend/src/shared/profiling"
)

func main() {
	databaseContext, cancelDatabaseContext := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancelDatabaseContext()

	startup := profiling.StartPhases(context.Background(), "startup")
	startup.Next("db_open")
	appDatabase, databaseOpenError := database.Open(databaseContext)
	if databaseOpenError != nil {
		panic(databaseOpenError)
	}
	startup.Next("migrate")
	if databaseMigrationError := appDatabase.Migrate(); databaseMigrationError != nil {
		panic(fmt.Errorf("migrate postgres schema: %w", databaseMigrationError))
	}

	// Load the OCR engines now, so the first upload does not pay for it.
	startup.Next("ocr_warmup")
	if warmUpError := ocr.WarmUp(); warmUpError != nil {
		panic(warmUpError)
	}
	startup.Next("routes")
	routes := app.Routes(appDatabase.Gorm)
	startup.End()
	// What actually ran, so a profile is never ambiguous about the OCR path.
	profiling.Start(context.Background(), "startup.info").
		Set("ocr_backend", ocr.Backend).Set("tesseract_version", ocr.Version()).
		Set("gomaxprocs", runtime.GOMAXPROCS(0)).Set("num_cpu", runtime.NumCPU()).
		Set("omp_thread_limit", os.Getenv("OMP_THREAD_LIMIT")).Set("go_version", runtime.Version()).End(nil)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	address := ":" + port
	fmt.Printf("Backend running on http://localhost%s\n", address)
	if serveError := http.ListenAndServe(address, routes); serveError != nil {
		panic(serveError)
	}
}
