package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"go.rtnl.ai/radish"
	"go.rtnl.ai/x/rlog"
)

var (
	wg         sync.WaitGroup
	logger     *slog.Logger
	tasks      *radish.Radish
	simulators []*Simulator
	logFile    *os.File
	logPath    string
	path       string
	name       string
	radishOnly bool
)

func main() {
	// Set up the flags and options for turnip integration testing.
	versionFlag := flag.Bool("version", false, "print the version and exit")

	// Bind configuration options to the flags.
	flag.BoolVar(&radishOnly, "radish-only", false, "only run the radish instance, do not run the simulators")
	flag.StringVar(&logPath, "log", "", "write logs to the specified file")
	flag.StringVar(&path, "path", "", "json configuration for task simulator(s)")
	flag.StringVar(&name, "name", "", "enter a unique name for the simulation (default is the host name)")

	// Set the usage function and parse command line arguments.
	flag.Usage = usage
	flag.Parse()

	if name == "" {
		name, _ = os.Hostname()
		name += "-" + strconv.Itoa(os.Getpid())
	}

	// Print the version and exit if the version flag is set.
	if *versionFlag {
		fmt.Printf("turnip %s\n", radish.Version(false))
		os.Exit(0)
	}

	if !radishOnly && path == "" {
		exitErr(errors.New("path to json simulator configuration is required"))
	}

	// Load the simulator configurations from the file.
	var err error
	if !radishOnly {
		if err = loadSimulators(); err != nil {
			exitErr(err)
		}
	}

	// Setup logging
	logging()

	// Create a new Radish instances, loading the configuration from the environment.
	if tasks, err = radish.New(nil); err != nil {
		exitErr(err)
	}

	// Register the workers.
	if err = radish.Register(tasks, radish.WorkFunc(Default)); err != nil {
		exitErr(err)
	}

	// Run the Radish instance.
	if err = tasks.Run(); err != nil {
		exitErr(err)
	}

	if radishOnly {
		// Run the tasks until CTRL+C is pressed.
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		return
	}

	// Create a cancel context for the shutdown signal.
	ctx, cancel := context.WithCancel(context.Background())

	// Setup signal handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
	}()

	logger.Info("starting simulations", "name", name, "count", len(simulators))
	for _, simulator := range simulators {
		wg.Add(1)
		go func(ctx context.Context, simulator *Simulator) {
			defer wg.Done()
			if err := Simulate(ctx, *simulator); err != nil {
				logger.Error("simulation error", "name", name, "error", err)
			}
		}(ctx, simulator)
	}

	wg.Wait()
	shutdown()
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	shutdown()
	os.Exit(1)
}

func usage() {
	// Get the default output (usually stderr)
	w := flag.CommandLine.Output()

	fmt.Fprintln(w, "Turnip is an integration testing framework for Radish.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "\n  turnip [-version]")
	fmt.Fprintln(w, "\nOptions:")
	flag.PrintDefaults()
}

func logging() {
	if logPath == "" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		var err error
		if logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err != nil {
			exitErr(err)
		}
		logger = slog.New(slog.NewJSONHandler(logFile, nil))
	}
	rlog.SetDefault(rlog.New(logger))
}

func loadSimulators() (err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return err
	}
	defer file.Close()

	simulators = make([]*Simulator, 0)
	if err = json.NewDecoder(file).Decode(&simulators); err != nil {
		return err
	}
	return nil
}

func shutdown() {
	logger.Info("shutting down", "name", name)
	if tasks != nil {
		tasks.Shutdown()
	}
	tasks = nil

	if logFile != nil {
		logFile.Close()
	}
	logFile = nil
	logger.Info("graceful shutdown complete", "name", name)
}
