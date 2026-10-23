package cli

import (
	"fmt"
	"text/tabwriter"

	"os"

	"github.com/MrBoggi/goTOV/internal/brewfather"
	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/MrBoggi/goTOV/internal/logger"
	"github.com/spf13/cobra"
)

var brewfatherBatchesCmd = &cobra.Command{
	Use:   "brewfather-batches",
	Short: "List alle batches fra Brewfather",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.New()

		// 1) Load config
		cfg, err := config.Load("")
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Brewfather.UserID == "" || cfg.Brewfather.APIKey == "" {
			return fmt.Errorf("Brewfather credentials mangler i config.yaml")
		}

		// 2) Init klient
		client := brewfather.NewClient(cfg.Brewfather.UserID, cfg.Brewfather.APIKey)

		// 3) Hent batcher
		batches, err := client.FetchBatches()
		if err != nil {
			return fmt.Errorf("fetch batches: %w", err)
		}

		// 4) Pen tabell
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "BATCH ID\tNAME")

		for _, b := range batches {
			// Brewfather setter nesten alltid batch.Name = "Batch" eller "Brygg",
			// men batch.Recipe.Name inneholder det faktiske ølnavnet.
			name := b.Name
			if b.Recipe.Name != "" {
				name = b.Recipe.Name
			}

			fmt.Fprintf(w, "%s\t%s\n", b.ID, name)
		}

		w.Flush()

		log.Info().
			Int("count", len(batches)).
			Msg("Brewfather batches loaded")

		return nil
	},
}

func init() {
	// registrer kommandoen
	rootCmd.AddCommand(brewfatherBatchesCmd)
}
