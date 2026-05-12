package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	remindb "github.com/special-place-administrator/remindb-Local-Hub/pkg/mcp"
	"github.com/special-place-administrator/remindb-Local-Hub/pkg/store"
	"github.com/special-place-administrator/remindb-Local-Hub/pkg/temperature"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	sourceDir      string
	rescanInterval time.Duration
	verbose        bool
	listenAddr     string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server with background rescan and temperature tracking",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&sourceDir, "source", "", "Source directory to watch for changes (falls back to REMINDB_SOURCE)")
	serveCmd.Flags().DurationVar(&rescanInterval, "rescan-interval", 0, "Rescan interval (e.g. 30s, 5m); 0 uses default (falls back to REMINDB_RESCAN_INTERVAL)")
	serveCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Emit debug-level logs (default level is info)")
	serveCmd.Flags().StringVar(&listenAddr, "listen", "", "TCP listen address for multi-client MCP transport; empty uses stdio")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	if err := applyServeEnv(cmd); err != nil {
		return err
	}

	logger := newServeLogger(verbose)

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	cfg := temperature.DefaultConfig()
	tracker, err := temperature.NewTracker(st, cfg, logger)
	if err != nil {
		return err
	}

	srv := remindb.NewServer(st, tracker, cfg,
		remindb.WithSourceDir(sourceDir),
		remindb.WithLogger(logger),
	)

	logger.Info("serve: starting",
		"db", dbPath,
		"source", sourceDir,
		"rescan_interval", rescanInterval,
		"tick_interval", cfg.TickInterval,
		"verbose", verbose,
		"version", version,
	)

	go checkLatestVersion(ctx, version, logger)

	if sourceDir != "" {
		if err := remindb.MaybeInitialCompile(ctx, st, sourceDir, logger); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)
	defer cancel()

	g.Go(func() error {
		defer cancel()
		return runMCPTransport(ctx, srv, logger)
	})
	g.Go(func() error {
		tracker.Run(ctx, func(ctx context.Context, nodes []*store.Node) {
			logger.Info("cold nodes detected", "count", len(nodes))
			tracker.MarkNotified(srv.NotifyColdNodes(ctx, nodes))
		})
		return nil
	})

	if sourceDir != "" {
		rescan, err := remindb.NewRescanLoop(st, sourceDir, rescanInterval, logger)
		if err != nil {
			return err
		}
		g.Go(func() error {
			rescan.Run(ctx)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		logger.Error("serve: stopped with error", "err", err)
		return err
	}
	logger.Info("serve: stopped")
	return nil
}

func runMCPTransport(ctx context.Context, srv *remindb.Server, logger *slog.Logger) error {
	if listenAddr == "" {
		return srv.Run(ctx)
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %s: %w", listenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	logger.Info("serve: MCP TCP listener ready", "addr", listenAddr)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				logger.Warn("serve: temporary accept error", "err", err)
				continue
			}
			return fmt.Errorf("failed to accept MCP client: %w", err)
		}

		go handleMCPConn(ctx, srv, logger, conn)
	}
}

func handleMCPConn(ctx context.Context, srv *remindb.Server, logger *slog.Logger, conn net.Conn) {
	logger.Info("serve: MCP client connected", "remote", conn.RemoteAddr().String())

	session, err := srv.Connect(ctx, &mcpsdk.IOTransport{Reader: conn, Writer: conn})
	if err != nil {
		logger.Warn("serve: failed to connect MCP client", "err", err)
		_ = conn.Close()
		return
	}

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	select {
	case <-ctx.Done():
		_ = session.Close()
		err = <-done
	case err = <-done:
	}

	if err != nil && ctx.Err() == nil {
		logger.Warn("serve: MCP client disconnected with error", "err", err)
	}
	_ = conn.Close()
}

func newServeLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func applyServeEnv(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("db") {
		if v := os.Getenv("REMINDB_DB"); v != "" {
			dbPath = v
			if err := absolutizeDBPath(); err != nil {
				return err
			}
		}
	}

	if sourceDir == "" {
		sourceDir = os.Getenv("REMINDB_SOURCE")
	}

	if !cmd.Flags().Changed("rescan-interval") {
		if v := os.Getenv("REMINDB_RESCAN_INTERVAL"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("failed to parse: REMINDB_RESCAN_INTERVAL=%q: %w", v, err)
			}
			rescanInterval = d
		}
	}
	return nil
}
