package bridge

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"

	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
)

var settingPath = regexp.MustCompile(`^[a-zA-Z0-9_]+\.[a-zA-Z0-9_]+$`)

type command struct {
	ctx    context.Context
	key    string
	value  []byte
	result chan error
	check  func() error
}

type commands chan command

func (queue commands) subscribe(ctx context.Context, logger *slog.Logger, client *mqtt.Client, prefix string) error {
	return client.Subscribe("set/+/+", 1, func(_ *mqtt.Client, message mqtt.Message) {
		if ctx.Err() != nil || message.Retained() {
			return
		}
		path, ok := strings.CutPrefix(message.Topic(), prefix+"/set/")
		if !ok {
			return
		}
		group, name, ok := strings.Cut(path, "/")
		if !ok {
			return
		}
		key, value := translatePower(group+"."+name, message.Payload())
		if err := queue.enqueue(command{ctx: ctx, key: key, value: value}); err != nil {
			logger.Warn("Ignored MQTT command", "error", err)
		}
	})
}

func translatePower(key string, value []byte) (string, []byte) {
	if key != "device.power_switch" {
		return key, value
	}
	switch string(value) {
	case "ON", "1":
		return "misc.start", []byte("1")
	case "OFF", "0", "false":
		return "misc.stop", []byte("1")
	default:
		return "", nil
	}
}

func (queue commands) enqueue(item command) error {
	if err := item.ctx.Err(); err != nil {
		return err
	}
	if !settingPath.MatchString(item.key) || len(item.value) == 0 || bytes.ContainsAny(item.value, "\r\n\x00\x02\x04") {
		return errors.New("invalid controller command")
	}
	item.value = bytes.Clone(item.value)
	select {
	case queue <- item:
		return nil
	default:
		return errors.New("controller command queue is busy")
	}
}

func (queue commands) write(ctx context.Context, key string, value []byte) error {
	return queue.writeChecked(ctx, key, value, nil)
}

func (queue commands) writeChecked(ctx context.Context, key string, value []byte, check func() error) error {
	item := command{ctx: ctx, key: key, value: value, result: make(chan error, 1), check: check}
	if err := queue.enqueue(item); err != nil {
		return err
	}
	select {
	case err := <-item.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (queue commands) run(ctx context.Context, logger *slog.Logger, boiler *nbe.NBE) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case item := <-queue:
			if ctx.Err() != nil {
				return
			}
			if item.check != nil {
				if err := item.check(); err != nil {
					item.result <- err
					continue
				}
			}
			_, err := boiler.Set(item.ctx, item.key, item.value)
			if item.result != nil {
				item.result <- err
			}
			if err != nil {
				logger.Warn("Controller command failed", "setting", item.key, "error", err)
			} else {
				logger.Info("Controller command accepted", "setting", item.key)
			}
		}
	}
}
