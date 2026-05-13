//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func platformUpdate() error {
	cmd := exec.Command("bash", "-c", "curl -fsSL "+installShellURL+" | bash")
	fmt.Fprintf(os.Stderr, "running: %s\n", cmd.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}
	return nil
}
