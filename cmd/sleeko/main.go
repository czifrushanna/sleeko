// sleeko runs the AI-powered Dockerfile optimizer as a pluggable Harbor
// optimizer adapter service. See pkg/adapter for the implementation.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/czifrushanna/sleeko/pkg/adapter"
)

func main() {
	cfg, err := adapter.LoadConfig()
	if err != nil {
		log.Fatalf("sleeko: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := adapter.NewServer(cfg)
	server.StartSweeper(ctx)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("sleeko %s listening on %s", adapter.Version, cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("sleeko: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("sleeko shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("sleeko shutdown: %v", err)
	}
}
