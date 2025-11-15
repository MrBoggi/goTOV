package cli

import (
	"github.com/MrBoggi/goTOV/internal/app"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start goTØV backend server",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.New()
		return app.RunServer(log)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
