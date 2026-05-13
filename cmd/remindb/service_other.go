//go:build !windows

package main

import "log/slog"

func isWindowsService() (bool, error) { return false, nil }

func runServeAsService(*slog.Logger) error { return nil }
