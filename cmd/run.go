package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

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
		executorFactory, err := executors.NewExecutorFactory(&config.ExecutorConfig, &config.ContainerExecutorConfig, logging.Logger)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		opts := &common.ExecutorOpts{
			WorkspaceDirectory: "/tmp",
		}
		executor, err := executorFactory.GetExecutor(uuid.MustParse("ccb51a13-2b4c-414b-abb0-430dcb40432b"), opts)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		var runningCmd common.RunningCommand
		if executor.Recovered() {
			runningCmd, err = recoveredExecutorRun(executor)
		} else {
			runningCmd, err = newExecutorRun(executor)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		logging.Logger.Infow(
			"result",
			"exitCode", runningCmd.StatusCode(),
			"finished", runningCmd.Finished(),
			"error", runningCmd.Error(),
			"killed", runningCmd.Killed(),
		)

		err = executor.Destroy()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func recoveredExecutorRun(executor common.Executor) (common.RunningCommand, error) {
	cmd := executor.GetRunningCommand(false)
	if cmd != nil {
		streams := &common.ExecutorStreams{
			OutputStream: os.Stdout,
		}
		return cmd, cmd.AttachWait(context.Background(), streams)

	} else {
		cmd = executor.GetPreviousRunningCommand(false)
		return cmd, nil
	}
}

func newExecutorRun(executor common.Executor) (common.RunningCommand, error) {
	err := executor.Prepare()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	script := `
max=$((SECONDS + 30))
count=0
while [[ ${SECONDS} -le ${max} ]]
do
	echo "I am alive $count"
	sleep 1
	(( count++ ))
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
	go timerRoutine(runnningCmd)
	err = runnningCmd.Wait()
	return runnningCmd, err
}

func timerRoutine(cmd common.RunningCommand) {
	timer1 := time.NewTimer(2 * time.Second)

	<-timer1.C
	cmd.Kill()
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
