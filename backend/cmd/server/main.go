// Command server runs the BuildCost API and its background job workers.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"buildcost/internal/app"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := app.OpenDB(ctx, env("DATABASE_URL", "postgres://buildcost@localhost:5544/buildcost"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	adminEmail := env("ADMIN_EMAIL", "analyst@buildcost.local")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "buildcost-dev"
		log.Printf("WARNING: ADMIN_PASSWORD is not set; a first staff account, if created now, uses the development password")
	}
	if err := app.Seed(ctx, db, adminEmail, adminPassword); err != nil {
		log.Fatalf("seed: %v", err)
	}

	dataDir, _ := filepath.Abs(env("DATA_DIR", "../.dev/files"))
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	s := &app.Server{DB: db, DataDir: dataDir}

	if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
		s.AI = app.NewAI(os.Getenv("AI_MODEL"))
		log.Printf("AI assistance enabled")
	} else {
		log.Printf("AI assistance disabled (ANTHROPIC_API_KEY not set); all deterministic features work without it")
	}

	python := env("PDF_WORKER_PYTHON", "../workers/.venv/bin/python")
	script := env("PDF_WORKER_SCRIPT", "../workers/extract_tables.py")
	if _, err := os.Stat(python); err == nil {
		if _, err := os.Stat(script); err == nil {
			s.PDFWorker = []string{python, script}
		}
	}
	if s.PDFWorker == nil {
		log.Printf("PDF table worker not found; PDF BOQ import is disabled (run `make worker`)")
	}

	for i := 0; i < 2; i++ {
		go s.RunWorker(ctx)
	}

	srv := &http.Server{Addr: env("ADDR", "127.0.0.1:8080"), Handler: s.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("BuildCost API listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
