package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/archive"
	"github.com/orvice/neo-line/internal/httpapi"
	"github.com/orvice/neo-line/internal/mcpserver"
	"github.com/orvice/neo-line/internal/scheduler"
	"github.com/orvice/neo-line/internal/store"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoStore, err := store.New(ctx)
	if err != nil {
		log.Fatalf("connect MongoDB: %v", err)
	}
	if err := mongoStore.EnsureAuthIndexes(ctx); err != nil {
		log.Fatalf("ensure auth indexes: %v", err)
	}
	if err := bootstrapAdmin(ctx, mongoStore); err != nil {
		log.Fatalf("bootstrap admin user: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := mongoStore.Close(shutdownCtx); err != nil {
			log.Printf("close MongoDB: %v", err)
		}
	}()

	archiver, archiveEnabled, err := archive.New(ctx, slog.Default())
	if err != nil {
		log.Fatalf("init archive: %v", err)
	}
	if archiveEnabled {
		log.Println("S3 check-result archiving enabled")
	}

	schedCtx, cancelSched := context.WithCancel(context.Background())
	archiveDone := make(chan struct{})

	config := &app.Config{
		Service: "neo-line",
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "pong"})
			})
			httpapi.Register(r, mongoStore)
			mcpserver.Register(r, mongoStore)
		},
		InitFunc: []func() error{
			func() error {
				go func() {
					archiver.Run(schedCtx)
					close(archiveDone)
				}()
				go scheduler.New(mongoStore, archiver).Start(schedCtx)
				return nil
			},
		},
		TeardownFunc: []func() error{
			func() error {
				cancelSched()
				// Wait for the archiver to drain and flush any buffered
				// results before the process exits.
				select {
				case <-archiveDone:
				case <-time.After(35 * time.Second):
					log.Println("archive flush timed out on shutdown")
				}
				return nil
			},
		},
	}

	application := app.New(config)
	application.Run()
}

// bootstrapAdmin initializes the admin account from the environment.
// ADMIN_PASSWORD is required to (re)set the admin credentials; ADMIN_EMAIL is
// optional and defaults to admin@neo-line.local. When ADMIN_PASSWORD is unset
// the admin account is left untouched.
func bootstrapAdmin(ctx context.Context, st store.Store) error {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		log.Println("ADMIN_PASSWORD not set, skipping admin user initialization")
		return nil
	}
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@neo-line.local"
	}
	if err := st.EnsureAdminUser(ctx, email, password); err != nil {
		return err
	}
	log.Printf("admin user ensured: %s", email)
	return nil
}
