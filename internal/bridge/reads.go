package bridge

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/controllerread"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
)

type readRequest struct {
	kind  string
	query url.Values
}

type controllerReads struct {
	boiler   controllerread.Controller
	client   *mqtt.Client
	logger   *slog.Logger
	gate     chan struct{}
	requests chan readRequest
	// Only the holder of gate reads or updates last.
	last map[string]time.Time
}

func newControllerReads(boiler controllerread.Controller, client *mqtt.Client, logger *slog.Logger) *controllerReads {
	return &controllerReads{boiler: boiler, client: client, logger: logger, gate: make(chan struct{}, 1),
		requests: make(chan readRequest, 16), last: make(map[string]time.Time)}
}

func (reads *controllerReads) subscribe(prefix string) error {
	return reads.client.Subscribe("get/controller/+", 1, func(_ *mqtt.Client, msg mqtt.Message) {
		kind := strings.TrimPrefix(msg.Topic(), prefix+"/get/controller/")
		if msg.Retained() || !controllerread.Known(kind) || len(msg.Payload()) > 4096 {
			return
		}
		q, err := url.ParseQuery(string(msg.Payload()))
		if err != nil {
			reads.logger.Debug("Invalid controller read query", "kind", kind)
			return
		}
		select {
		case reads.requests <- readRequest{kind, q}:
		default:
			reads.logger.Warn("Controller read queue full", "kind", kind)
		}
	})
}

func (reads *controllerReads) read(ctx context.Context, kind string, q url.Values) (any, error) {
	return reads.refresh(ctx, kind, q, 0)
}

func (reads *controllerReads) refresh(ctx context.Context, kind string, q url.Values, minimumAge time.Duration) (any, error) {
	if !controllerread.Known(kind) {
		return nil, controllerread.ErrKind
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reads.gate <- struct{}{}:
	}
	defer func() { <-reads.gate }()
	if minimumAge > 0 && time.Since(reads.last[kind]) < minimumAge {
		return nil, nil
	}
	data, err := controllerread.Read(ctx, reads.boiler, kind, q)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	topic := kind
	if kind == "events" && q.Get("at") != "" {
		topic = "event_history"
	}
	result := map[string]any{"read_at": time.Now().UTC(), "query": q.Encode()}
	if err == nil {
		result["data"] = data
	} else {
		result["error"] = controllerread.Message(kind, err)
	}
	updates := map[string]any{topic: result}
	if kind == "versions" || kind == "sensors" {
		count := any("None")
		if err == nil {
			switch rows := data.(type) {
			case []nbe.ProgramVersion:
				count = len(rows)
			case []nbe.RemoteSensor:
				count = len(rows)
			}
		}
		updates[kind+"_count"] = count
	}
	if pubErr := reads.client.PublishMany("controller", updates); pubErr != nil {
		reads.logger.Warn("Could not queue controller read result", "kind", kind, "error", pubErr)
	} else if len(q) == 0 && !errors.Is(err, controllerread.ErrQuery) {
		reads.last[kind] = time.Now()
	}
	return data, err
}

func (reads *controllerReads) run(ctx context.Context) {
	// Poll one list per tick. Interactive reads also refresh the list's age.
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	lists := []struct {
		kind     string
		interval time.Duration
	}{
		{"versions", time.Hour}, {"sensors", time.Minute}, {"events", time.Minute},
	}
	next := 0
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case req := <-reads.requests:
			_, _ = reads.read(ctx, req.kind, req.query)
		case <-tick.C:
			list := lists[next]
			_, _ = reads.refresh(ctx, list.kind, nil, list.interval)
			next = (next + 1) % len(lists)
		}
	}
}
