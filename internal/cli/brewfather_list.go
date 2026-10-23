package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/MrBoggi/goTOV/internal/brewfather"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/spf13/cobra"
)

var brewfatherListCmd = &cobra.Command{
	Use:   "brewfather-list",
	Short: "List all Brewfather recipes available in API",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.New()

		// 1) Load config
		cfg, err := config.Load("")
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Brewfather.UserID == "" || cfg.Brewfather.APIKey == "" {
			return fmt.Errorf("Brewfather credentials missing in config.yaml")
		}

		// 2) Create client
		client := brewfather.NewClient(cfg.Brewfather.UserID, cfg.Brewfather.APIKey)

		// 3) Fetch list
		list, err := client.ListRecipes()
		if err != nil {
			return fmt.Errorf("fetch recipe list: %w", err)
		}

		// 4) Pretty print as table
		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME")

		for _, r := range list {
			fmt.Fprintf(w, "%s\t%s\n", r.ID, r.Name)
		}

		w.Flush()

		log.Info().Int("recipes", len(list)).Msg("Brewfather recipe list loaded")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(brewfatherListCmd)
}
