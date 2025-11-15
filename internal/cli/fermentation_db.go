package cli

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/MrBoggi/goTOV/internal/fermentation"
	"github.com/spf13/cobra"
)

var fermentationDBCmd = &cobra.Command{
	Use:   "fermentation-db",
	Short: "Administrer lokal fermentasjonsdatabase",
}

var fermentationListPlansCmd = &cobra.Command{
	Use:   "plans",
	Short: "List alle fermenteringsplaner",
	RunE: func(cmd *cobra.Command, args []string) error {

		store, err := fermentation.NewSQLiteStore("data/fermentation.db")
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer store.Close()

		plans, err := store.ListPlans()
		if err != nil {
			return fmt.Errorf("list plans: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTEPS")

		for _, p := range plans {
			fmt.Fprintf(w, "%d\t%s\t%d\n", p.ID, p.Name, p.TotalSteps)
		}
		w.Flush()

		return nil
	},
}

var fermentationListStepsCmd = &cobra.Command{
	Use:   "steps <plan_id>",
	Short: "Vis alle fermenterings-steg for en plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		planID, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid plan id: %w", err)
		}

		store, err := fermentation.NewSQLiteStore("data/fermentation.db")
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer store.Close()

		steps, err := store.ListSteps(planID)
		if err != nil {
			return fmt.Errorf("list steps: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "STEP\tTEMP (C)\tHOURS\tTYPE\tDESCRIPTION")

		for _, s := range steps {
			fmt.Fprintf(w, "%d\t%.1f\t%.1f\t%s\t%s\n",
				s.StepNumber, s.Temperature, s.DurationHours, s.Type, s.Description)
		}
		w.Flush()

		return nil
	},
}

var fermentationDBClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Slett ALLE fermenteringsplaner (ADVARSEL)",
	RunE: func(cmd *cobra.Command, args []string) error {

		store, err := fermentation.NewSQLiteStore("data/fermentation.db")
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer store.Close()

		if err := store.Clear(); err != nil {
			return fmt.Errorf("clear db: %w", err)
		}

		fmt.Println("✔ Fermentation DB cleared")
		return nil
	},
}

func init() {
	fermentationDBCmd.AddCommand(fermentationListPlansCmd)
	fermentationDBCmd.AddCommand(fermentationListStepsCmd)
	fermentationDBCmd.AddCommand(fermentationDBClearCmd)

	rootCmd.AddCommand(fermentationDBCmd)
}
