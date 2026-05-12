package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultBridgeAddr = "127.0.0.1:39291"

var (
	bridgeAddr           string
	bridgeSourceDir      string
	bridgeRescanInterval time.Duration
	bridgeStartupTimeout time.Duration
)

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Bridge stdio MCP clients to a singleton local remindb serve process",
	RunE:  runBridge,
}

func init() {
	bridgeCmd.Flags().StringVar(&bridgeAddr, "addr", defaultBridgeAddr, "Local TCP address for the singleton remindb MCP server")
	bridgeCmd.Flags().StringVar(&bridgeSourceDir, "source", "", "Source directory to watch for changes (falls back to REMINDB_SOURCE)")
	bridgeCmd.Flags().DurationVar(&bridgeRescanInterval, "rescan-interval", 0, "Rescan interval passed to the singleton server (falls back to REMINDB_RESCAN_INTERVAL)")
	bridgeCmd.Flags().DurationVar(&bridgeStartupTimeout, "startup-timeout", 10*time.Second, "How long to wait for the singleton server to start")
	rootCmd.AddCommand(bridgeCmd)
}

func runBridge(cmd *cobra.Command, _ []string) error {
	if err := applyBridgeEnv(cmd); err != nil {
		return err
	}

	conn, err := connectOrStartBridgeDaemon(bridgeAddr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	return bridgeStdio(conn)
}

func applyBridgeEnv(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("db") {
		if v := os.Getenv("REMINDB_DB"); v != "" {
			dbPath = v
			if err := absolutizeDBPath(); err != nil {
				return err
			}
		}
	}

	if !cmd.Flags().Changed("addr") {
		if v := os.Getenv("REMINDB_BRIDGE_ADDR"); v != "" {
			bridgeAddr = v
		}
	}

	if bridgeSourceDir == "" {
		bridgeSourceDir = os.Getenv("REMINDB_SOURCE")
	}

	if !cmd.Flags().Changed("rescan-interval") {
		if v := os.Getenv("REMINDB_RESCAN_INTERVAL"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("failed to parse: REMINDB_RESCAN_INTERVAL=%q: %w", v, err)
			}
			bridgeRescanInterval = d
		}
	}

	return nil
}

func connectOrStartBridgeDaemon(addr string) (net.Conn, error) {
	conn, err := dialBridge(addr)
	if err == nil {
		return conn, nil
	}

	if startErr := startBridgeDaemon(addr); startErr != nil {
		return nil, startErr
	}

	deadline := time.Now().Add(bridgeStartupTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, lastErr = dialBridge(addr)
		if lastErr == nil {
			return conn, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to connect to remindb singleton at %s after %s: %w", addr, bridgeStartupTimeout, lastErr)
}

func dialBridge(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, 500*time.Millisecond)
}

func startBridgeDaemon(addr string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate executable: %w", err)
	}

	args := []string{"serve", "--listen", addr, "--db", dbPath}
	if bridgeSourceDir != "" {
		args = append(args, "--source", bridgeSourceDir)
	}
	if bridgeRescanInterval > 0 {
		args = append(args, "--rescan-interval", bridgeRescanInterval.String())
	}

	logFile, err := openBridgeDaemonLog(addr)
	if err != nil {
		return err
	}

	daemon := exec.Command(exe, args...)
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	daemon.Stdin = nil

	if err := daemon.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("failed to start remindb singleton: %w", err)
	}

	_, _ = fmt.Fprintf(logFile, "started remindb singleton pid=%d addr=%s db=%s source=%s\n", daemon.Process.Pid, addr, dbPath, bridgeSourceDir)
	if err := daemon.Process.Release(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("failed to release remindb singleton: %w", err)
	}
	_ = logFile.Close()
	return nil
}

func openBridgeDaemonLog(addr string) (*os.File, error) {
	safeAddr := strings.NewReplacer(":", "-", "\\", "-", "/", "-", ".", "-").Replace(addr)
	path := filepath.Join(os.TempDir(), "remindb-singleton-"+safeAddr+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open daemon log: %s: %w", path, err)
	}
	return f, nil
}

func bridgeStdio(conn net.Conn) error {
	stdinDone := make(chan error, 1)
	stdoutDone := make(chan error, 1)
	go func() {
		err := copyStdinToConn(conn, os.Stdin)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		stdinDone <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		stdoutDone <- err
	}()

	select {
	case err := <-stdoutDone:
		_ = conn.Close()
		return normalizeCopyErr(err)
	case err := <-stdinDone:
		if err := normalizeCopyErr(err); err != nil {
			_ = conn.Close()
			return err
		}
		return normalizeCopyErr(<-stdoutDone)
	}
}

func copyStdinToConn(dst io.Writer, src io.Reader) error {
	r := bufio.NewReader(src)
	if prefix, err := r.Peek(3); err == nil && bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = r.Discard(3)
	}
	_, err := io.Copy(dst, r)
	return err
}

func normalizeCopyErr(err error) error {
	if err == nil || err == io.EOF || err == net.ErrClosed {
		return nil
	}
	return err
}
