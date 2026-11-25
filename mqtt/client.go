/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021-2023 Mark Lipscombe.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, version 3.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package mqtt

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	connectTimeout   = 5 * time.Second
	ackTimeout       = 5 * time.Second
	maxRetryDelay    = 10 * time.Second
	publishBatchSize = 16
)

type Client struct {
	prefix string
	logger *slog.Logger

	mu            sync.Mutex
	subscriptions map[string]*subscriptionInfo
	watch         *TopicWatch
	retained      map[string][]byte
	changed       chan struct{}
	cancel        context.CancelFunc
	done          chan struct{}
}

type subscriptionInfo struct {
	qos      byte
	callback MessageHandler
	optional bool
}

// TopicWatch follows external topics on the bridge's MQTT connection. Its
// subscriptions are independent of the boiler topic prefix.
type TopicWatch struct {
	client       *Client
	subscription *subscriptionInfo
	topics       map[string]*subscriptionInfo
	disconnected func()
}

type Message mqtt.Message
type MessageHandler func(client *Client, message Message)

// Broker holds validated connection options for the MQTT broker.
type Broker struct {
	options *mqtt.ClientOptions
}

// Open starts connecting in the background so polling can continue through a broker outage.
func (broker *Broker) Open(ctx context.Context, logger *slog.Logger, clientID, prefix string) *Client {
	ctx, cancel := context.WithCancel(ctx)
	client := &Client{
		prefix:        prefix,
		logger:        logger,
		subscriptions: make(map[string]*subscriptionInfo),
		retained:      map[string][]byte{prefix + "/device/controller_status": []byte("offline")},
		changed:       make(chan struct{}, 1),
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	options := *broker.options
	options.SetClientID(clientID)
	options.SetWill(prefix+"/device/status", "offline", 1, true)
	go client.run(ctx, options)
	return client
}

func (client *Client) Close() {
	client.cancel()
	<-client.done
}

func (client *Client) PublishMany(topic string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	updates := make(map[string][]byte, len(values))
	for key, value := range values {
		name := client.prefix + "/" + topic + "/" + key
		payload, err := rawPayload(value)
		if err != nil {
			return fmt.Errorf("marshal MQTT payload for %s: %w", name, err)
		}
		updates[name] = payload
	}
	client.mu.Lock()
	maps.Copy(client.retained, updates)
	client.mu.Unlock()
	client.notifyChanged()
	return nil
}

func rawPayload(value any) ([]byte, error) {
	switch payload := value.(type) {
	case string:
		return []byte(payload), nil
	case []byte:
		return bytes.Clone(payload), nil
	default:
		return json.Marshal(value)
	}
}

func (client *Client) PublishRaw(topic string, value any) error {
	payload, err := rawPayload(value)
	return client.publish(topic, payload, err)
}

func (client *Client) PublishJSON(topic string, value any) error {
	payload, err := json.Marshal(value)
	return client.publish(topic, payload, err)
}

func (client *Client) publish(topic string, payload []byte, err error) error {
	if err != nil {
		return fmt.Errorf("marshal MQTT payload for %s: %w", topic, err)
	}
	client.mu.Lock()
	client.retained[topic] = payload
	client.mu.Unlock()
	client.notifyChanged()
	return nil
}

// Subscribe registers a subscription to install on this and subsequent connections.
func (client *Client) Subscribe(topic string, qos byte, callback MessageHandler) error {
	if topic == "" || qos > 2 || callback == nil {
		return fmt.Errorf("invalid MQTT subscription for %q", topic)
	}
	client.mu.Lock()
	client.subscriptions[client.prefix+"/"+topic] = &subscriptionInfo{qos: qos, callback: callback}
	client.mu.Unlock()
	client.notifyChanged()
	return nil
}

func (client *Client) WatchTopics(callback MessageHandler, disconnected func()) *TopicWatch {
	watch := &TopicWatch{client: client, disconnected: disconnected,
		subscription: &subscriptionInfo{qos: 1, callback: callback, optional: true}}
	client.mu.Lock()
	client.watch = watch
	client.mu.Unlock()
	client.notifyChanged()
	return watch
}

func (watch *TopicWatch) SetTopics(topics []string) {
	updated := make(map[string]*subscriptionInfo, len(topics))
	for _, topic := range topics {
		updated[topic] = watch.subscription
	}
	client := watch.client
	client.mu.Lock()
	changed := !maps.Equal(watch.topics, updated)
	watch.topics = updated
	client.mu.Unlock()
	if changed {
		client.notifyChanged()
	}
}

func (client *Client) notifyChanged() {
	select {
	case client.changed <- struct{}{}:
	default:
	}
}

func (client *Client) run(ctx context.Context, opts mqtt.ClientOptions) {
	defer close(client.done)

	retryDelay := time.Second
	for ctx.Err() == nil {
		// Each attempt gets a clean Paho client. Only our latest retained values
		// are replayed, never an old queue of in-flight publishes.
		lost := make(chan error, 1)
		opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			select {
			case lost <- err:
			default:
			}
		})
		connection := mqtt.NewClient(&opts)
		err := waitForToken(ctx, connection.Connect(), 2*connectTimeout+time.Second)
		if err == nil {
			connectedAt := time.Now()
			err = client.serve(ctx, connection, lost)
			if time.Since(connectedAt) >= maxRetryDelay {
				retryDelay = time.Second
			}
			if ctx.Err() != nil && connection.IsConnectionOpen() {
				offline := connection.Publish(client.prefix+"/device/status", 1, true, "offline")
				offline.WaitTimeout(time.Second)
			}
			connection.Disconnect(250)
		} else if err == context.DeadlineExceeded || ctx.Err() != nil {
			connection.Disconnect(250)
		}
		if ctx.Err() != nil {
			return
		}

		client.logger.Warn("MQTT session failed", "error", err, "retry_in", retryDelay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
			retryDelay = min(2*retryDelay, maxRetryDelay)
		}
	}
}

func (client *Client) serve(ctx context.Context, connection mqtt.Client, lost <-chan error) error {
	subscribed := make(map[string]*subscriptionInfo)
	sent := make(map[string][]byte)
	online := false
	var callbacks sync.Mutex
	closed := false
	defer func() {
		callbacks.Lock()
		defer callbacks.Unlock()
		closed = true
		client.mu.Lock()
		watch := client.watch
		client.mu.Unlock()
		if watch != nil {
			watch.disconnected()
		}
	}()
	for {
		client.mu.Lock()
		subscriptions := maps.Clone(client.subscriptions)
		if client.watch != nil {
			for topic, sub := range client.watch.topics {
				if subscriptions[topic] == nil {
					subscriptions[topic] = sub
				}
			}
		}
		retained := maps.Clone(client.retained)
		client.mu.Unlock()

		for topic := range subscribed {
			if subscriptions[topic] == nil {
				if err := waitForToken(ctx, connection.Unsubscribe(topic), ackTimeout); err != nil {
					return fmt.Errorf("unsubscribe from %s: %w", topic, err)
				}
				delete(subscribed, topic)
			}
		}
		for topic, sub := range subscriptions {
			if subscribed[topic] == sub {
				continue
			}
			token := connection.Subscribe(topic, sub.qos, func(_ mqtt.Client, msg mqtt.Message) {
				if sub.optional {
					callbacks.Lock()
					defer callbacks.Unlock()
					if closed {
						return
					}
				}
				sub.callback(client, msg)
			})
			if err := waitForToken(ctx, token, ackTimeout); err != nil {
				return fmt.Errorf("subscribe to %s: %w", topic, err)
			}
			if granted, ok := token.(*mqtt.SubscribeToken).Result()[topic]; !ok || granted > 2 {
				if !sub.optional {
					return fmt.Errorf("MQTT broker rejected subscription to %s", topic)
				}
				client.logger.Warn("MQTT broker rejected external topic subscription", "topic", topic)
			}
			subscribed[topic] = sub
		}

		if err := publishRetained(ctx, connection, retained, sent); err != nil {
			return err
		}

		if !online {
			token := connection.Publish(client.prefix+"/device/status", 1, true, "online")
			if err := waitForToken(ctx, token, ackTimeout); err != nil {
				return fmt.Errorf("publish MQTT availability: %w", err)
			}
			client.logger.Info("MQTT connected")
			online = true
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-lost:
			return err
		case <-client.changed:
		}
	}
}

func publishRetained(ctx context.Context, connection mqtt.Client, retained, sent map[string][]byte) error {
	type publication struct {
		topic   string
		payload []byte
		token   mqtt.Token
	}
	var updates []publication
	for topic, payload := range retained {
		if previous, ok := sent[topic]; !ok || !bytes.Equal(previous, payload) {
			updates = append(updates, publication{topic: topic, payload: payload})
		}
	}
	for start := 0; start < len(updates); start += publishBatchSize {
		batch := updates[start:min(start+publishBatchSize, len(updates))]
		for i := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			batch[i].token = connection.Publish(batch[i].topic, 1, true, batch[i].payload)
		}
		for _, item := range batch {
			if err := waitForToken(ctx, item.token, ackTimeout); err != nil {
				return fmt.Errorf("publish %s: %w", item.topic, err)
			}
			sent[item.topic] = item.payload
		}
	}
	return nil
}

func waitForToken(ctx context.Context, token mqtt.Token, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return context.DeadlineExceeded
	case <-token.Done():
		return token.Error()
	}
}

// PrepareBroker validates the broker configuration without opening a connection.
func PrepareBroker(uri *url.URL) (*Broker, error) {
	opts := mqtt.NewClientOptions()
	if uri == nil {
		return nil, fmt.Errorf("MQTT broker URL is required")
	}
	host := uri.Hostname()
	if host == "" {
		return nil, fmt.Errorf("MQTT broker hostname is required")
	}

	port := uri.Port()
	switch uri.Scheme {
	case "mqtts":
		if port == "" {
			port = "8883"
		}
		query := uri.Query()
		tlsCert := query.Get("tls_cert")
		tlsKey := query.Get("tls_key")
		caCert := query.Get("tls_cacert")
		if caCert == "" {
			caCert = query.Get("tls_ca")
		}

		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Retain the explicit legacy opt-out for privately managed brokers.
			InsecureSkipVerify: query.Get("insecure") == "true",
		}
		if (tlsCert == "") != (tlsKey == "") {
			return nil, fmt.Errorf("MQTT TLS client certificate and key must be supplied together")
		}
		if tlsCert != "" {
			cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
			if err != nil {
				return nil, fmt.Errorf("could not load the MQTT TLS certificate and key")
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		if caCert != "" {
			caCertData, err := os.ReadFile(caCert)
			if err != nil {
				return nil, fmt.Errorf("could not read the MQTT CA certificate")
			}
			tlsConfig.RootCAs = x509.NewCertPool()
			if !tlsConfig.RootCAs.AppendCertsFromPEM(caCertData) {
				return nil, fmt.Errorf("MQTT CA certificate contains no valid certificates")
			}
		}

		opts.SetTLSConfig(tlsConfig)
		opts.AddBroker("ssl://" + net.JoinHostPort(host, port))
	case "mqtt", "tcp":
		if port == "" {
			port = "1883"
		}
		opts.AddBroker("tcp://" + net.JoinHostPort(host, port))
	default:
		return nil, fmt.Errorf("unsupported MQTT URL scheme %q", uri.Scheme)
	}

	opts.SetUsername(uri.User.Username())
	password, _ := uri.User.Password()
	opts.SetPassword(password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetDialer(&net.Dialer{Timeout: connectTimeout})
	opts.SetWriteTimeout(ackTimeout)
	opts.SetKeepAlive(15 * time.Second)
	opts.SetPingTimeout(5 * time.Second)
	return &Broker{options: opts}, nil
}
