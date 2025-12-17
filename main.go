package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Config configures the server
const (
	Port           = ":8080"
	ReadTimeout    = 5 * time.Second
	WriteTimeout   = 5 * time.Second
	MaxHeaderBytes = 1 << 20 // 1 MB
)

// redactAuth returns a safe version of the Authorization header
func redactAuth(token string) string {
	if token == "" {
		return ""
	}
	if len(token) < 15 {
		return "REDACTED"
	}
	// Show first 10 chars (covers "Bearer " + a bit) and last 3 chars
	return token[:10] + "..." + token[len(token)-3:]
}

func main() {
	// 1. Setup Structured Logger (JSON)
	// perfect for ingestion into security SIEMs or Loki
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("Starting HTTP Mirror Sink", "port", Port)

	// 2. Define the Handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Calculate Request Line Size
		// Method + SP + URI + SP + Proto + CRLF
		requestLineSize := int64(len(r.Method) + 1 + len(r.RequestURI) + 1 + len(r.Proto) + 2)

		// Calculate Header Size
		headerSize := int64(0)
		for k, v := range r.Header {
			// Key + ": " + Value + CRLF
			// If there are multiple values for a key, they are usually joined or sent as separate lines.
			// Go's Header map has []string values. We'll assume standard wire format approximation.
			for _, val := range v {
				headerSize += int64(len(k) + 2 + len(val) + 2)
			}
		}
		// End of Headers CRLF
		headerSize += 2

		// READ FLOW SIZE:
		// We must read the body to know the size, but we discard the content
		// to keep memory footprint low.
		bytesReceived, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			logger.Error("Failed to read body", "error", err)
		}
		defer r.Body.Close()

		totalRequestSize := requestLineSize + headerSize + bytesReceived
		duration := time.Since(start)

		// 3. Log Metadata
		// We log the request details, headers of interest, and flow stats.
		logger.Info("request_mirrored",
			slog.Group("flow",
				slog.Int64("total_request_size", totalRequestSize),
				slog.Int64("header_size", headerSize),
				slog.Int64("body_size", bytesReceived),
				slog.Duration("latency", duration),
				slog.String("client_ip", r.RemoteAddr),
			),
			slog.Group("http",
				slog.String("method", r.Method),
				slog.String("host", r.Host),
				slog.String("uri", r.RequestURI),
				slog.String("proto", r.Proto),
				slog.String("user_agent", r.UserAgent()),
				slog.String("referer", r.Referer()),
			),
			slog.Group("metadata",
				slog.String("x_forwarded_for", r.Header.Get("X-Forwarded-For")),
				slog.String("x_request_id", r.Header.Get("X-Request-Id")),
				slog.String("x_envoy_original_path", r.Header.Get("X-Envoy-Original-Path")),
				slog.String("authorization", redactAuth(r.Header.Get("Authorization"))),
				slog.Bool("has_cookie", r.Header.Get("Cookie") != ""),
			),
		)

		// 4. Respond with Nothing
		// The Gateway API ignores this response, but we send a 200 OK
		// to complete the TCP handshake cleanly.
		w.WriteHeader(http.StatusOK)
	})

	// 3. Server Configuration
	srv := &http.Server{
		Addr:           Port,
		Handler:        mux,
		ReadTimeout:    ReadTimeout,
		WriteTimeout:   WriteTimeout,
		MaxHeaderBytes: MaxHeaderBytes,
	}

	// 4. Graceful Shutdown Boilerplate
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exiting")
}
