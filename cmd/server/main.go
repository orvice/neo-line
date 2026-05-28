package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"butterfly.orx.me/core/app"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/httpapi"
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
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := mongoStore.Close(shutdownCtx); err != nil {
			log.Printf("close MongoDB: %v", err)
		}
	}()

	schedCtx, cancelSched := context.WithCancel(context.Background())

	config := &app.Config{
		Service: "neo-line",
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "pong"})
			})
			httpapi.Register(r, mongoStore)
		},
		InitFunc: []func() error{
			func() error {
				go scheduler.New(mongoStore).Start(schedCtx)
				return nil
			},
		},
		TeardownFunc: []func() error{
			func() error {
				cancelSched()
				return nil
			},
		},
	}

	application := app.New(config)
	application.Run()
}
