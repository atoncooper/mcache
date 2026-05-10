package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/spf13/cobra"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/infra"
	"github.com/atoncooper/mcache/mbr"
	"github.com/atoncooper/mcache/monitor"
	mnet "github.com/atoncooper/mcache/net"
)

var serverConfigPath string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the mcache TCP server",
	Long:  `Start the mcache TCP server with the given configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := mcache.LoadConfig(serverConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: load config: %v\n", err)
			os.Exit(1)
		}

		logger := initLogger(cfg.Server.Logging)

		opts, err := cfg.Cache.BuildOptions()
		if err != nil {
			logger.Error("build cache options", map[string]any{"error": err.Error()})
			os.Exit(1)
		}

		// Wire infra observer if enabled - logs cache hit/miss/set/del/evict/rehash.
		var infraObs *infra.Infra
		if cfg.Cache.ObserverEnabled {
			infraOpts := []infra.Option{infra.WithLoggerInstance(logger)}

			if cfg.Server.Metrics.Enabled {
				infraOpts = append(infraOpts, infra.WithPrometheus(true))
			}

			infraObs = infra.New(infraOpts...)
			opts = opts.WithObserver(infraObs)

			if cfg.Server.Metrics.Enabled && infraObs != nil {
				if err := infraObs.RegisterPrometheus(prometheus.DefaultRegisterer); err != nil {
					logger.Error("prometheus register", map[string]any{"error": err.Error()})
				}
			}

			logger.Debug("observer wired", map[string]any{
				"observer_enabled":  true,
				"prometheus_enabled": cfg.Server.Metrics.Enabled,
			})
		}

		c, err := mcache.New(opts)
		if err != nil {
			logger.Error("create cache", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
		defer c.Close()

		readTimeout, _ := time.ParseDuration(cfg.Server.ReadTimeout)
		if readTimeout == 0 {
			readTimeout = 30 * time.Second
		}
		writeTimeout, _ := time.ParseDuration(cfg.Server.WriteTimeout)
		gracefulTimeout, _ := time.ParseDuration(cfg.Server.GracefulShutdownTimeout)

		srv := mnet.NewServer(c,
			mnet.WithWorkers(cfg.Server.Workers),
			mnet.WithMaxConns(cfg.Server.MaxConns),
			mnet.WithReadTimeout(readTimeout),
			mnet.WithWriteTimeout(writeTimeout),
			mnet.WithGracefulShutdownTimeout(gracefulTimeout),
			mnet.WithErrorLog(func(format string, v ...any) {
				logger.Error(fmt.Sprintf(format, v...), nil)
			}),
			mnet.WithInfoLog(func(format string, v ...any) {
				logger.Info(fmt.Sprintf(format, v...), nil)
			}),
		)

		go func() {
			logger.Info("server listening", map[string]any{"address": cfg.Server.Address})
			if err := srv.ListenAndServe(cfg.Server.Address); err != nil {
				logger.Error("server exited", map[string]any{"error": err.Error()})
			}
		}()

		// Start Prometheus metrics HTTP server if enabled.
		var metricsSrv *http.Server
		if cfg.Server.Metrics.Enabled {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			metricsSrv = &http.Server{
				Addr:    cfg.Server.Metrics.Address,
				Handler: mux,
			}
			go func() {
				logger.Info("metrics server listening", map[string]any{"address": cfg.Server.Metrics.Address})
				if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("metrics server", map[string]any{"error": err.Error()})
				}
			}()
		}

		// Start MBR decision engine if enabled.
		if cfg.MBR.Enabled {
			startMBR(c, cfg, logger)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutting down", nil)

		if metricsSrv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsSrv.Shutdown(ctx); err != nil {
				logger.Error("metrics shutdown", map[string]any{"error": err.Error()})
			}
		}

		if infraObs != nil {
			infraObs.Stop()
		}

		if err := srv.Close(); err != nil {
			logger.Error("server close", map[string]any{"error": err.Error()})
		}
	},
}

func init() {
	serverCmd.Flags().StringVar(&serverConfigPath, "config", "config.yaml", "path to configuration file")
	rootCmd.AddCommand(serverCmd)
}

// startMBR initialises and starts the MBR decision engine.
func startMBR(c *mcache.Cache, cfg *mcache.Config, logger infra.Logger) {
	interval, _ := time.ParseDuration(cfg.MBR.DecisionInterval)
	if interval == 0 {
		interval = 500 * time.Millisecond
	}

	// Build MBR options
	mbrOpts := mbr.NewOptions().
		WithMatrixCapacity(cfg.MBR.MatrixCapacity).
		WithDecisionInterval(interval).
		WithPID(mbr.PIDConfig{
			Kp:       cfg.MBR.PID.Kp,
			Ki:       cfg.MBR.PID.Ki,
			Kd:       cfg.MBR.PID.Kd,
			Setpoint: cfg.MBR.Setpoint,
			Min:      -1.0,
			Max:      1.0,
		}).
		WithWeights(mbr.ScoreWeights{
			MemGrowth:        cfg.MBR.Weights.MemGrowth,
			HitRate:          cfg.MBR.Weights.HitRate,
			NewKeys:          cfg.MBR.Weights.NewKeys,
			EvictionPressure: cfg.MBR.Weights.EvictionPressure,
			BufferPenalty:    cfg.MBR.Weights.BufferPenalty,
		}).
		WithMigration(mbr.DefaultMigratorConfig())

	// Start system monitor
	mon := monitor.New(monitor.NewOptions().
		WithInterval(5 * time.Second).
		WithCapacity(60).
		WithCollectors(monitor.NewRuntime()),
	)
	mon.Start()

	// Create PID controller
	pid := mbr.NewPIDController(mbrOpts.PID)

	// Create feature matrix
	matrix := mbr.NewFeatureMatrix(mbrOpts.MatrixCapacity)

	// Create feature provider
	provider := mbr.NewDefaultStatsProvider(c, mon, pid)

	// Inject provider observer into cache (compose with existing observer if needed)
	c.SetObserver(provider.Observer())

	// Decision channel
	decisionCh := make(chan mbr.DecisionEvent, 16)

	// Context for orderly shutdown
	ctx := context.Background()

	// Start decision loop
	go mbr.RunDecisionLoop(ctx, provider, matrix, decisionCh,
		mbr.WithInterval(mbrOpts.DecisionInterval),
		mbr.WithMatrixCapacity(mbrOpts.MatrixCapacity),
		mbr.WithPID(mbrOpts.PID),
		mbr.WithWeights(mbrOpts.Weights),
		mbr.WithMigratorConfig(mbrOpts.Migration),
	)

	// Start migration executor
	go mbr.RunMigrationExecutor(ctx, decisionCh, c, mon, provider, mbrOpts.Migration)

	logger.Info("MBR decision engine started", map[string]any{
		"matrix_capacity": mbrOpts.MatrixCapacity,
		"interval":        mbrOpts.DecisionInterval.String(),
		"setpoint":        mbrOpts.PID.Setpoint,
	})
}
