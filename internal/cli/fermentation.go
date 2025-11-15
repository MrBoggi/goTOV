package cli

import (
	"fmt"

	"github.com/MrBoggi/goTOV/internal/brewfather"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/spf13/cobra"
)

var fermentationImportCmd = &cobra.Command{
	Use:   "fermentation-import <brewfather-batch-id>",
	Short: "Importer gjæringsprofil fra Brewfather-batch til lokal fermentasjonsdatabase",
	Args:  cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		batchID := args[0]
		log := logger.New()

		// 1) Last config
		cfg, err := config.Load("")
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Brewfather.UserID == "" || cfg.Brewfather.APIKey == "" {
			return fmt.Errorf("Brewfather credentials mangler i config.yaml")
		}

		// 2) Init klient
		bfClient := brewfather.NewClient(cfg.Brewfather.UserID, cfg.Brewfather.APIKey)

		// 3) Hent batch
		batch, err := bfClient.FetchBatch(batchID)
		if err != nil {
			return fmt.Errorf("fetch Brewfather batch %s: %w", batchID, err)
		}

		// 4) Konverter til fermentasjonsplan
		plan, err := brewfather.ExtractFermentationPlanFromBatch(batch)
		if err != nil {
			return fmt.Errorf("extract fermentation plan: %w", err)
		}

		// 5) Lagre i lokal SQLite
		const dbPath = "data/fermentation.db"
		store, err := fermentation.NewSQLiteStore(dbPath)
		if err != nil {
			return fmt.Errorf("open fermentation db %s: %w", dbPath, err)
		}
		defer store.Close()

		if _, err := store.SavePlan(*plan); err != nil {
			return fmt.Errorf("save fermentation plan: %w", err)
		}

		log.Info().
			Str("batch_id", batchID).
			Str("name", plan.Name).
			Int("steps", plan.TotalSteps).
			Msg("✅ Fermentation plan imported successfully")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(fermentationImportCmd)
}
