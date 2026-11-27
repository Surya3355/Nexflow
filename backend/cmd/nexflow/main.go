// Command nexflow runs the Nexflow API server and its scheduled jobs.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/config"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/alerts"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/bankqueries"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/handover"
	"github.com/Surya3355/nexflow/backend/modules/history"
	apihttp "github.com/Surya3355/nexflow/backend/modules/http"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/officers"
	"github.com/Surya3355/nexflow/backend/modules/reports"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		if err := runCreateAdmin(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openAppPool(ctx, cfg.DBURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	rec := history.NewPgRecorder(pool)
	store, err := documents.NewLocalStore(cfg.UploadDir)
	if err != nil {
		log.Fatal(err)
	}
	queue := alerts.NewQueue(pool)
	accessSvc := access.NewService(pool, rec)
	accessSvc.SetReassigner(handover.New(rec, queue))
	wf := workflows.NewService(pool, rec)
	caseSvc := cases.NewService(pool, wf, rec)
	caseSvc.SetNotifier(queue)
	leadSvc := leads.NewService(pool, rec)
	leadSvc.SetNotifier(queue)
	archiveSvc := archive.NewService(pool, rec)
	archiveSvc.SetFileRemover(store)

	docs := documents.NewService(pool, store, rec)
	auth := access.NewHandler(accessSvc, access.NewSessionManager(pool, cfg.SecureCookies))
	auth.TrustProxy(cfg.TrustProxy)
	handler := apihttp.NewRouter(apihttp.Deps{
		Auth:           auth,
		Access:         accessSvc,
		Workflows:      wf,
		Customers:      customers.NewService(pool, rec),
		Cases:          caseSvc,
		Documents:      docs,
		Linker:         documents.NewLinker(cfg.LinkSecret),
		Archive:        archiveSvc,
		Reports:        reports.NewService(pool),
		Leads:          leadSvc,
		Officers:       officers.NewService(pool, wf, rec),
		Queries:        bankqueries.NewService(pool, docs, rec),
		AllowedOrigins: cfg.AllowedOrigins,
	})

	jobs := Jobs{Archive: archiveSvc, Queue: queue, Sender: unconfiguredSender{}}
	go schedule(ctx, jobs, time.Now)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("nexflow listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// openAppPool runs the migrations as the database owner, then opens the pool
// the app serves from, which acts as the app role and can't change history.
func openAppPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	owner, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	err = migrateUp(owner)
	owner.Close()
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	migrations.UseAppRole(cfg)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return pool, nil
}
