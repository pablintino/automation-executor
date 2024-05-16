package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors"
	"github.com/pablintino/automation-executor/internal/executors/common"
	"github.com/pablintino/automation-executor/logging"

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
		defer logging.Logger.Sync()

		config, err := config.Configure()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		executorFactory, err := executors.NewExecutorFactory(&config.ExecutorConfig, &config.ContainerExecutorConfig)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		opts := &common.ExecutorOpts{
			WorkspaceDirectory: "/tmp",
		}
		executor, err := executorFactory.GetExecutor(uuid.New(), opts)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = executor.Prepare()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		script := `
max=$((SECONDS + 2))

while [[ ${SECONDS} -le ${max} ]]
do
	echo "I am alive"
	sleep 2
done
`
		execCommand := &common.ExecutorCommand{
			ImageName: "docker.io/library/debian:bookworm",
			IsSupport: false,
			Script:    script,
			Command:   []string{"/bin/bash"},
		}
		streams := &common.ExecutorStreams{
			OutputStream: os.Stdout,
		}
		runnningCmd, err := executor.Execute(context.Background(), execCommand, streams)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = runnningCmd.Wait()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		logging.Logger.Infow("result", "exitCode", runnningCmd.StatusCode(), "finished", runnningCmd.Finished(), "error", runnningCmd.Error())

		err = executor.Destroy()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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
