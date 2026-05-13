package main

import (
	"github.com/spf13/cobra"
)

const (
	installShellURL = "https://raw.githubusercontent.com/special-place-administrator/remindb-Local-Hub/main/install.sh"
	installPSURL    = "https://raw.githubusercontent.com/special-place-administrator/remindb-Local-Hub/main/install.ps1"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Reinstall remindb by re-running the install script from main",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cobraCmd *cobra.Command, _ []string) error {
	cobraCmd.SilenceUsage = true
	return platformUpdate()
}
