package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/mlipscombe/boiler-mate/config"
	"github.com/mlipscombe/boiler-mate/homeassistant"
	"github.com/mlipscombe/boiler-mate/internal/webui"
	"github.com/mlipscombe/boiler-mate/monitor"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Run(parent context.Context, cfg config.Config, logger *slog.Logger) error {
	broker, err := mqtt.PrepareBroker(cfg.MQTT)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	boiler, err := nbe.Open(ctx, cfg.Controller)
	if err != nil {
		return err
	}
	defer boiler.Close()
	logger.Info("Controller connected", "host", cfg.Controller.Host, "serial", boiler.Serial)
	prefix := mqttPrefix(cfg.MQTT, boiler.Serial)

	queue := make(commands, 32)
	httpErrors := make(chan error, 1)
	var workers sync.WaitGroup
	var ui *webui.UI
	if cfg.WebBind != "false" {
		var write webui.WriteFunc
		if pin, _ := cfg.Controller.User.Password(); pin != "" {
			write = queue.write
		}
		ui, err = webui.New(ctx, logger, boiler, boiler.Serial, cfg.WebData, write)
		if err != nil {
			return err
		}
		defer func() {
			if err := ui.Close(); err != nil {
				logger.Warn("Could not close web history", "error", err)
			}
		}()
		if err := serve(ctx, &workers, logger, cfg.WebBind, "web", ui.Handler(), httpErrors); err != nil {
			return err
		}
	}

	client := broker.Open(ctx, logger, "nbemqtt-"+boiler.Serial, prefix)
	defer client.Close()
	defer func() {
		cancel()
		workers.Wait()
	}()
	if err := queue.subscribe(ctx, logger, client, prefix); err != nil {
		return err
	}
	workers.Go(func() {
		queue.run(ctx, logger, boiler)
	})
	if err := client.PublishMany("device", map[string]any{"serial": boiler.Serial, "ip_address": boiler.IPAddress}); err != nil {
		return err
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	if cfg.Bind != "false" {
		if err := serve(ctx, &workers, logger, cfg.Bind, "metrics", metricsHandler(registry), nil); err != nil {
			logger.Error("Could not start metrics server", "error", err)
		}
	}

	var observe monitor.Observer
	if ui != nil {
		observe = func(topic string, values map[string]any) {
			ui.Observe(ctx, topic, values)
		}
		workers.Go(func() { ui.Run(ctx) })
	}
	var ready func()
	if cfg.HADiscovery {
		ready = func() {
			if err := homeassistant.PublishDiscovery(client, boiler.Serial, prefix); err != nil {
				logger.Warn("Could not queue Home Assistant discovery", "error", err)
			}
		}
	}
	workers.Go(func() {
		monitor.Run(ctx, logger, boiler, client, registry, boiler.Serial, observe, ready)
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-httpErrors:
		return err
	}
}

func mqttPrefix(uri *url.URL, serial string) string {
	if len(uri.Path) > 1 {
		return uri.Path[1:]
	}
	return "nbe/" + serial
}

func metricsHandler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	metrics := promhttp.InstrumentMetricHandler(registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.Handle("/metrics", metrics)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"healthy":true}`)
	})
	mux.HandleFunc("/liveness", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "OK")
	})
	return mux
}

func serve(ctx context.Context, workers *sync.WaitGroup, logger *slog.Logger, address, name string, handler http.Handler, fatal chan<- error) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listen for %s: %w", name, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	logger.Info("HTTP server listening", "server", name, "address", listener.Addr())
	workers.Go(func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if fatal != nil {
				fatal <- fmt.Errorf("serve %s: %w", name, err)
			} else {
				logger.Error("HTTP server stopped", "server", name, "error", err)
			}
		}
	})
	workers.Go(func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Warn("HTTP shutdown timed out", "server", name, "error", err)
			_ = server.Close()
		}
	})
	return nil
}
