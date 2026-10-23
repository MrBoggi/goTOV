package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd er topp-kommandoen: `gotov`
var rootCmd = &cobra.Command{
	Use:   "gotov",
	Short: "goTØV brewery backend & tools",
	Long:  "goTØV backend: OPC UA bridge, web API and tooling (Brewfather import, fermentation, etc.)",
}

// Execute kjøres fra cmd/gotov/main.go
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// init() i denne pakken brukes til å registrere subcommands
func init() {
	// subcommands legges til i andre filer via rootCmd.AddCommand(...)
}
