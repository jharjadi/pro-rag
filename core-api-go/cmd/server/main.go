package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jharjadi/pro-rag/core-api-go/internal/config"
	"github.com/jharjadi/pro-rag/core-api-go/internal/db"
	"github.com/jharjadi/pro-rag/core-api-go/internal/handler"
	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Connect to database with retry
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Run startup checks (extensions + tables)
	if err := db.StartupChecks(ctx, pool); err != nil {
		slog.Error("startup checks failed", "error", err)
		os.Exit(1)
	}

	// Initialize services
	retrievalSvc := service.NewRetrievalService(pool)
	rerankerSvc := service.NewRerankerService(
		cfg.CohereAPIKey,
		cfg.CohereRerankerModel,
		cfg.RerankTimeout(),
		cfg.RerankMaxDocs,
		cfg.RerankFailOpen,
	)
	llmSvc := service.NewLLMService(
		cfg.LLMProvider,
		cfg.LLMModel,
		cfg.AnthropicAPIKey,
		cfg.LLMMaxTokens,
	)
	embedSvc := service.NewEmbedService(cfg.EmbedEndpoint)

	// Initialize handlers
	queryHandler := handler.NewQueryHandler(cfg, retrievalSvc, rerankerSvc, llmSvc, embedSvc)

	// Build router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"unhealthy","error":"%s"}`, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Query endpoint
	r.Post("/v1/query", queryHandler.Handle)

	// Serve web UI (static files from /web directory if it exists)
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "/web"
	}
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		slog.Info("serving web UI", "dir", webDir)
		fs := http.FileServer(http.Dir(webDir))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Serve index.html for root path
			if r.URL.Path == "/" {
				http.ServeFile(w, r, webDir+"/index.html")
				return
			}
			fs.ServeHTTP(w, r)
		})
	} else {
		slog.Info("web UI not available", "dir", webDir, "reason", "directory not found")
	}

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting server", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdownCtx.Done()
	slog.Info("shutting down server...")

	cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(cancelCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
