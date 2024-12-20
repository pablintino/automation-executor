package cmd

import (
	"fmt"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/db"
	"github.com/pablintino/automation-executor/internal/services/secrets"
	"github.com/pablintino/automation-executor/logging"
	"os"

	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		logging.Initialize(true)
		defer func() {
			if err := logging.Logger.Sync(); err != nil {
				fmt.Println(err)
			}
		}()

		config, err := config.Configure()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		sqlDb, err := db.NewSQLDatabase(&config.DatabaseConfig)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		svc := secrets.NewSecretsService(sqlDb.Secrets(), logging.Logger)
		model, err := svc.AddSecretToken("test-secret", "secret-value")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println(model)

	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
