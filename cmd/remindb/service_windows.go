//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	defaultServiceName        = "remindb-singleton"
	defaultServiceDisplayName = "ReminDB-Local-Hub singleton"
	defaultServiceDescription = "ReminDB-Local-Hub singleton MCP server listening on loopback for shared MCP clients."
)

var (
	serviceInstallListen    string
	serviceInstallSource    string
	serviceInstallRescan    string
	serviceInstallStartType string
	serviceInstallLogFile   string
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the Windows Service for remindb-singleton (Windows only)",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Register the Windows Service and start it",
	RunE:  runServiceInstall,
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and unregister the Windows Service",
	RunE:  runServiceUninstall,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Windows Service",
	RunE:  runServiceStart,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Windows Service",
	RunE:  runServiceStop,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report Windows Service status",
	RunE:  runServiceStatus,
}

func init() {
	serviceInstallCmd.Flags().StringVar(&serviceInstallListen, "listen", "127.0.0.1:39291", "TCP listen address passed to remindb serve")
	serviceInstallCmd.Flags().StringVar(&serviceInstallSource, "source", "", "Source directory passed to remindb serve")
	serviceInstallCmd.Flags().StringVar(&serviceInstallRescan, "rescan-interval", "60s", "Rescan interval passed to remindb serve (empty omits the flag)")
	serviceInstallCmd.Flags().StringVar(&serviceInstallStartType, "start-type", "auto-delayed", "Service start type: auto-delayed | auto | manual | disabled")
	serviceInstallCmd.Flags().StringVar(&serviceInstallLogFile, "log-file", "", "Log file path baked into the service args (default: C:\\ProgramData\\remindb\\service.log)")

	serviceCmd.AddCommand(serviceInstallCmd, serviceUninstallCmd, serviceStartCmd, serviceStopCmd, serviceStatusCmd)
	rootCmd.AddCommand(serviceCmd)
}

func runServiceInstall(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	if dbPath == "" || strings.EqualFold(filepath.Base(dbPath), "memory.db") && !filepath.IsAbs(dbPath) {
		return fmt.Errorf("--db is required for service install (pass an absolute path)")
	}
	if !filepath.IsAbs(dbPath) {
		abs, err := filepath.Abs(dbPath)
		if err != nil {
			return fmt.Errorf("failed to resolve: --db: %w", err)
		}
		dbPath = abs
	}
	if serviceInstallSource != "" && !filepath.IsAbs(serviceInstallSource) {
		abs, err := filepath.Abs(serviceInstallSource)
		if err != nil {
			return fmt.Errorf("failed to resolve: --source: %w", err)
		}
		serviceInstallSource = abs
	}
	if serviceInstallLogFile == "" {
		serviceInstallLogFile = filepath.Join(os.Getenv("ProgramData"), "remindb", "service.log")
	}
	if err := os.MkdirAll(filepath.Dir(serviceInstallLogFile), 0o755); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect SCM: %w", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(defaultServiceName); err == nil {
		_ = existing.Close()
		return fmt.Errorf("service %q is already installed; run `remindb service uninstall` first", defaultServiceName)
	}

	args := []string{
		"--db", dbPath,
		"serve",
		"--listen", serviceInstallListen,
		"--log-file", serviceInstallLogFile,
	}
	if serviceInstallSource != "" {
		args = append(args, "--source", serviceInstallSource)
	}
	if serviceInstallRescan != "" {
		args = append(args, "--rescan-interval", serviceInstallRescan)
	}

	startType, delayedAuto, err := parseStartType(serviceInstallStartType)
	if err != nil {
		return err
	}

	cfg := mgr.Config{
		DisplayName:      defaultServiceDisplayName,
		Description:      defaultServiceDescription,
		StartType:        startType,
		DelayedAutoStart: delayedAuto,
		ErrorControl:     mgr.ErrorNormal,
	}

	s, err := m.CreateService(defaultServiceName, exe, cfg, args...)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("service registered but failed to start: %w", err)
	}

	fmt.Printf("service %q installed and started (listen=%s db=%s source=%s log=%s)\n",
		defaultServiceName, serviceInstallListen, dbPath, serviceInstallSource, serviceInstallLogFile)
	return nil
}

func parseStartType(s string) (uint32, bool, error) {
	switch strings.ToLower(s) {
	case "auto-delayed":
		return mgr.StartAutomatic, true, nil
	case "auto":
		return mgr.StartAutomatic, false, nil
	case "manual":
		return mgr.StartManual, false, nil
	case "disabled":
		return mgr.StartDisabled, false, nil
	default:
		return 0, false, fmt.Errorf("unknown --start-type %q (expected auto-delayed, auto, manual, disabled)", s)
	}
}

func runServiceUninstall(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(defaultServiceName)
	if err != nil {
		return fmt.Errorf("failed to open service %q: %w", defaultServiceName, err)
	}
	defer s.Close()

	if st, qerr := s.Query(); qerr == nil && st.State != svc.Stopped {
		_, _ = s.Control(svc.Stop)
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			st, qerr = s.Query()
			if qerr != nil || st.State == svc.Stopped {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	fmt.Printf("service %q uninstalled\n", defaultServiceName)
	return nil
}

func runServiceStart(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	return withService(func(s *mgr.Service) error {
		if err := s.Start(); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}
		fmt.Printf("service %q start signal sent\n", defaultServiceName)
		return nil
	})
}

func runServiceStop(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	return withService(func(s *mgr.Service) error {
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		fmt.Printf("service %q stop signal sent\n", defaultServiceName)
		return nil
	})
}

func runServiceStatus(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	return withService(func(s *mgr.Service) error {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service: %w", err)
		}
		cfg, _ := s.Config()
		fmt.Printf("name:        %s\n", defaultServiceName)
		fmt.Printf("state:       %s\n", stateName(st.State))
		fmt.Printf("start_type:  %s\n", startTypeName(cfg.StartType, cfg.DelayedAutoStart))
		fmt.Printf("executable:  %s\n", cfg.BinaryPathName)
		if cfg.Description != "" {
			fmt.Printf("description: %s\n", cfg.Description)
		}
		return nil
	})
}

func withService(fn func(*mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect SCM: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(defaultServiceName)
	if err != nil {
		return fmt.Errorf("failed to open service %q: %w", defaultServiceName, err)
	}
	defer s.Close()
	return fn(s)
}

func stateName(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start-pending"
	case svc.StopPending:
		return "stop-pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue-pending"
	case svc.PausePending:
		return "pause-pending"
	case svc.Paused:
		return "paused"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

func startTypeName(t uint32, delayed bool) string {
	switch t {
	case mgr.StartAutomatic:
		if delayed {
			return "auto-delayed"
		}
		return "auto"
	case mgr.StartManual:
		return "manual"
	case mgr.StartDisabled:
		return "disabled"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func isWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func runServeAsService(logger *slog.Logger) error {
	if err := svc.Run(defaultServiceName, &remindbService{logger: logger}); err != nil {
		return fmt.Errorf("service dispatch failed: %w", err)
	}
	return nil
}

type remindbService struct {
	logger *slog.Logger
}

func (rs *remindbService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- serveCore(ctx, rs.logger) }()

	status <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case c := <-requests:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-serveErr:
				case <-time.After(30 * time.Second):
				}
				return false, 0
			}
		case err := <-serveErr:
			status <- svc.Status{State: svc.StopPending}
			if err != nil {
				if rs.logger != nil {
					rs.logger.Error("service: serve core exited", "err", err)
				}
				return true, 1
			}
			return false, 0
		}
	}
}
