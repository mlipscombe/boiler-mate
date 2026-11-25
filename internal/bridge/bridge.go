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
	"strconv"
	"sync"
	"time"

	"github.com/mlipscombe/boiler-mate/config"
	"github.com/mlipscombe/boiler-mate/homeassistant"
	"github.com/mlipscombe/boiler-mate/internal/heating"
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

	pin, _ := cfg.Controller.User.Password()
	writesEnabled := cfg.EnableWrites && pin != ""
	if cfg.EnableWrites && !writesEnabled {
		logger.Warn("Boiler writes require a controller PIN")
	}

	var queue commands
	if writesEnabled {
		queue = make(commands, 32)
	}
	httpErrors := make(chan error, 1)
	var workers sync.WaitGroup
	client := broker.Open(ctx, logger, "nbemqtt-"+boiler.Serial, prefix)
	defer client.Close()
	reads := newControllerReads(boiler, client, logger)
	var writeHeating heating.WriteFunc
	if writesEnabled {
		writeHeating = func(ctx context.Context, key string, value int, current func() bool) error {
			return queue.writeChecked(ctx, key, []byte(strconv.Itoa(value)), func() error {
				if !current() {
					return heating.ErrSuperseded
				}
				if key == heating.MaxPower {
					return checkHeatingPower(ctx, boiler, value, current)
				}
				return nil
			})
		}
	}
	external := heating.New(cfg.WebData, boiler.Serial, logger, heating.Options{
		Write: writeHeating, DashboardReadings: cfg.WebBind != "false", ThermostatUnit: cfg.MQTTThermostatUnit,
	})
	watchHeating := cfg.WebBind != "false" || external.Enabled()
	var manualHeating *heating.Control
	if watchHeating {
		manualHeating = external
		watch := client.WatchTopics(func(_ *mqtt.Client, message mqtt.Message) {
			external.Receive(message.Topic(), message.Payload(), message.Retained())
		}, external.Disconnected)
		external.Watch(watch.SetTopics)
	}
	var ui *webui.UI
	if cfg.WebBind != "false" {
		var write webui.WriteFunc
		if writesEnabled {
			write = func(ctx context.Context, key string, value []byte) error {
				return queue.write(ctx, key, value, manualHeating)
			}
		}
		ui, err = webui.New(ctx, logger, boiler, boiler.Serial, cfg.WebData, webui.Options{
			ReadController: reads.read, Write: write, OutdoorWeatherFahrenheit: cfg.OutdoorWeatherFahrenheit,
			Heating: external,
		})
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

	defer func() {
		cancel()
		workers.Wait()
	}()
	if err := reads.subscribe(prefix); err != nil {
		return err
	}
	workers.Go(func() { reads.run(ctx) })
	if writesEnabled {
		if err := queue.subscribe(ctx, logger, client, prefix, manualHeating); err != nil {
			return err
		}
		workers.Go(func() {
			queue.run(ctx, logger, boiler)
		})
	}
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
	if watchHeating {
		observe = external.Observe
		workers.Go(func() { external.Run(ctx) })
	}
	if ui != nil {
		observe = func(topic string, values map[string]any) {
			external.Observe(topic, values)
			ui.Observe(ctx, topic, values)
		}
		workers.Go(func() { ui.Run(ctx) })
	}
	var ready func()
	if cfg.HADiscovery {
		firstPoll := make(chan struct{})
		ready = func() { close(firstPoll) }
		workers.Go(func() {
			select {
			case <-ctx.Done():
			case <-firstPoll:
				publishDiscovery(ctx, logger, boiler, client, prefix)
			}
		})
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

func publishDiscovery(ctx context.Context, logger *slog.Logger, boiler *nbe.NBE, client *mqtt.Client, prefix string) {
	for ctx.Err() == nil {
		response, err := boiler.Get(ctx, nbe.GetSetupRangeFunction, "boiler.*")
		if ctx.Err() != nil {
			return
		}
		var ranges map[string]any
		if err == nil {
			ranges = response.Payload
		} else {
			logger.Debug("Could not read boiler ranges for Home Assistant", "error", err)
		}
		if err := homeassistant.PublishDiscovery(client, boiler.Serial, prefix, ranges); err != nil {
			logger.Warn("Could not queue Home Assistant discovery", "error", err)
		}
		if err == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func mqttPrefix(uri *url.URL, serial string) string {
	if len(uri.Path) > 1 {
		return uri.Path[1:]
	}
	return "nbe/" + serial
}

func checkHeatingPower(ctx context.Context, boiler *nbe.NBE, value int, current func() bool) error {
	response, err := boiler.Get(ctx, nbe.GetSetupRangeFunction, heating.MaxPower)
	if err != nil {
		return errors.New("could not read the controller's maximum-power range")
	}
	bounds := nbe.ReadSettingRange(response.Payload, "boiler_power_max")
	if bounds == nil || float64(value) < bounds.Min || float64(value) > bounds.Max {
		return errors.New("the band is outside the controller's maximum-power range")
	}
	response, err = boiler.Get(ctx, nbe.GetSetupFunction, "regulation.*")
	if err != nil {
		return errors.New("could not read the controller's current burner limits")
	}
	switch minimum := response.Payload["boiler_power_min"].(type) {
	case int64:
		if int64(value) < minimum {
			return errors.New("the band is below the controller's minimum burner power")
		}
	case nbe.RoundedFloat:
		if nbe.RoundedFloat(value) < minimum {
			return errors.New("the band is below the controller's minimum burner power")
		}
	default:
		return errors.New("the controller did not report its minimum burner power")
	}
	if !current() {
		return heating.ErrSuperseded
	}
	return nil
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
