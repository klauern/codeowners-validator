package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mszostok/codeowners-validator/internal/check"
	"github.com/mszostok/codeowners-validator/internal/load"
	"github.com/mszostok/codeowners-validator/internal/runner"
	"github.com/mszostok/codeowners-validator/pkg/codeowners"
	"github.com/mszostok/codeowners-validator/pkg/version"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// Config holds the application configuration
type Config struct {
	RepositoryPath     string
	CheckFailureLevel  string
	Checks             *cli.StringSlice
	ExperimentalChecks *cli.StringSlice
}

func main() {
	version.Init()
	if version.ShouldPrintVersion() {
		version.PrintVersion(os.Stdout)
		os.Exit(0)
	}

	var cfg Config
	app := &cli.App{
		Name:                 "codeowners-validator",
		Usage:                "",
		Version:              version.Get().Version,
		EnableBashCompletion: false,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "path",
				Usage:       "repository path to validate the CODEOWNERS file",
				EnvVars:     []string{"REPOSITORY_PATH"},
				Value:       ".",
				Destination: &cfg.RepositoryPath,
			},
			&cli.StringFlag{
				Name:        "fail-level",
				Usage:       "minimum severity needed to fail the check run",
				EnvVars:     []string{"CHECK_FAILURE_LEVEL"},
				Value:       "WARNING",
				Destination: &cfg.CheckFailureLevel,
			},
			&cli.StringSliceFlag{
				Name:        "checks",
				Usage:       "list of checks to perform on the file",
				EnvVars:     []string{"CHECKS"},
				Value:       cli.NewStringSlice("files", "duppatterns"),
				Destination: cfg.Checks,
			},
			&cli.StringSliceFlag{
				Name:        "experiments",
				Usage:       "list of experimental checks to perform",
				EnvVars:     []string{"EXPERIMENTAL_CHECKS"},
				Value:       cli.NewStringSlice("notowned"),
				Destination: cfg.ExperimentalChecks,
			},
		},
		Action: cfg.runValidations,
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}

}

func (cfg *Config) runValidations(c *cli.Context) error {
	log := logrus.New()

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	cancelOnInterrupt(ctx, cancelFunc)

	// init codeowners entries
	codeownersEntries, err := codeowners.NewFromPath(cfg.RepositoryPath)
	if err != nil {
		return err
	}

	// init checks
	checks, err := load.Checks(ctx, cfg.Checks.Value(), cfg.ExperimentalChecks.Value())
	if err != nil {
		return err
	}

	// run check runner
	absRepoPath, err := filepath.Abs(cfg.RepositoryPath)
	if err != nil {
		return err
	}

	severity := check.Warning
	severity.Set(cfg.CheckFailureLevel)
	checkRunner := runner.NewCheckRunner(log, codeownersEntries, absRepoPath, severity, checks...)
	checkRunner.Run(ctx)

	if ctx.Err() != nil {
		log.Error("Application was interrupted by operating system")
		os.Exit(2)
	}
	if checkRunner.ShouldExitWithCheckFailure() {
		os.Exit(3)
	}
	return nil
}

// cancelOnInterrupt calls cancel func when os.Interrupt or SIGTERM is received
func cancelOnInterrupt(ctx context.Context, cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-c:
			cancel()
			<-c
			os.Exit(1) // second signal. Exit directly.
		}
	}()
}
