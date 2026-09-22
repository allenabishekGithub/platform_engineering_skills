package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	nafmockv1 "github.com/allenabishekGithub/platform_engineering_skills/gateway-week/mock-grpcServer/api/nafmock/v1"
	"github.com/allenabishekGithub/platform_engineering_skills/gateway-week/mock-grpcServer/internal/server"
)

func main() {
	grpcAddr := flag.String("grpc-addr", ":50052", "gRPC listen address")
	httpAddr := flag.String("http-addr", ":9091", "admin HTTP listen address (healthz, metrics, history, behavior, reset)")
	behavior := flag.String("behavior", "ok", "default behavior for all operations")
	historyLimit := flag.Int("history-limit", 100, "max in-memory history entries")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	reg := prometheus.NewRegistry()
	srv, err := server.New(reg, *behavior, *historyLimit)
	if err != nil {
		slog.Error("invalid default behavior", "err", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	nafmockv1.RegisterNAFMockServer(grpcServer, srv)
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer)

	httpSrv := &http.Server{
		Addr:              *httpAddr,
		Handler:           srv.AdminMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		slog.Error("grpc listen failed", "addr", *grpcAddr, "err", err)
		os.Exit(1)
	}
	go func() {
		slog.Info("admin http listening", "addr", *httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin http failed", "err", err)
			os.Exit(1)
		}
	}()
	go func() {
		healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		slog.Info("nafmock listening", "grpc_addr", *grpcAddr, "behavior", *behavior)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("grpc serve failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	grpcServer.GracefulStop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	slog.Info("stopped")
}
