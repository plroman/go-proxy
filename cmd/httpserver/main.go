package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashbots/orderflow-proxy/common"
	"github.com/flashbots/orderflow-proxy/httpserver"
	"github.com/flashbots/orderflow-proxy/proxy"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2" // imports as package "cli"
)

var flags []cli.Flag = []cli.Flag{
	&cli.StringFlag{
		Name:  "internal-listen-addr",
		Value: "127.0.0.1:8080",
		Usage: "address to listen on for internal API",
	},
	&cli.StringFlag{
		Name:  "metrics-addr",
		Value: "127.0.0.1:8090",
		Usage: "address to listen on for Prometheus metrics",
	},
	&cli.BoolFlag{
		Name:  "log-json",
		Value: false,
		Usage: "log in JSON format",
	},
	&cli.BoolFlag{
		Name:  "log-debug",
		Value: false,
		Usage: "log debug messages",
	},
	&cli.BoolFlag{
		Name:  "log-uid",
		Value: false,
		Usage: "generate a uuid and add to all log messages",
	},
	&cli.StringFlag{
		Name:  "log-service",
		Value: "your-project",
		Usage: "add 'service' tag to logs",
	},
	&cli.BoolFlag{
		Name:  "pprof",
		Value: false,
		Usage: "enable pprof debug endpoint",
	},
	&cli.Int64Flag{
		Name:  "drain-seconds",
		Value: 45,
		Usage: "seconds to wait in drain HTTP request",
	},
	&cli.StringFlag{
		Name:  "listen-addr",
		Value: "127.0.0.1:9090",
		Usage: "address to listen on for orderflow proxy API",
	},
	&cli.StringFlag{
		Name:  "builder-endpoint",
		Value: "127.0.0.1:8546",
		Usage: "address to send local ordeflow to",
	},
	&cli.DurationFlag{
		Name:  "cert-duration",
		Value: time.Hour * 24 * 365,
		Usage: "generated certificate duration",
	},
	&cli.StringSliceFlag{
		Name:  "cert-hosts",
		Value: cli.NewStringSlice("127.0.0.1", "localhost"),
		Usage: "generated certificate hosts",
	},
}

func main() {
	app := &cli.App{
		Name:  "orderflow-proxy",
		Usage: "Serve API, and metrics",
		Flags: flags,
		Action: func(cCtx *cli.Context) error {
			internalListenAddr := cCtx.String("internal-listen-addr")
			metricsAddr := cCtx.String("metrics-addr")
			logJSON := cCtx.Bool("log-json")
			logDebug := cCtx.Bool("log-debug")
			logUID := cCtx.Bool("log-uid")
			logService := cCtx.String("log-service")
			enablePprof := cCtx.Bool("pprof")
			drainDuration := time.Duration(cCtx.Int64("drain-seconds")) * time.Second

			log := common.SetupLogger(&common.LoggingOpts{
				Debug:   logDebug,
				JSON:    logJSON,
				Service: logService,
				Version: common.Version,
			})

			if logUID {
				id := uuid.Must(uuid.NewRandom())
				log = log.With("uid", id.String())
			}

			cfg := &httpserver.HTTPServerConfig{
				ListenAddr:  internalListenAddr,
				MetricsAddr: metricsAddr,
				Log:         log,
				EnablePprof: enablePprof,

				DrainDuration:            drainDuration,
				GracefulShutdownDuration: 30 * time.Second,
				ReadTimeout:              60 * time.Second,
				WriteTimeout:             30 * time.Second,
			}

			srv, err := httpserver.New(cfg)
			if err != nil {
				cfg.Log.Error("failed to create server", "err", err)
				return err
			}

			exit := make(chan os.Signal, 1)
			signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
			srv.RunInBackground()

			builderEndpoint := cCtx.String("builder-endpoint")
			listedAddr := cCtx.String("listen-addr")
			certDuration := cCtx.Duration("cert-duration")
			certHosts := cCtx.StringSlice("cert-hosts")
			proxyConfig := &proxy.Config{
				Log:               log,
				MetricsServer:     srv.MetricsSrv,
				BuilderEndpoint:   builderEndpoint,
				ListenAddr:        listedAddr,
				CertValidDuration: certDuration,
				CertHosts:         certHosts,
			}

			proxy, err := proxy.New(*proxyConfig)
			if err != nil {
				cfg.Log.Error("failed to create proxy server", "err", err)
				return err
			}
			err = proxy.RunProxyInBackground()
			if err != nil {
				cfg.Log.Error("failed to start proxy server", "err", err)
				return err
			}

			<-exit

			// Shutdown server once termination signal is received
			srv.Shutdown()
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
