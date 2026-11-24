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
	"maps"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

const (
	connectTimeout   = 5 * time.Second
	ackTimeout       = 5 * time.Second
	maxRetryDelay    = 10 * time.Second
	publishBatchSize = 16
)

type Client struct {
	URI      *url.URL
	ClientID string
	Prefix   string

	mu            sync.Mutex
	subscriptions map[string]*subscriptionInfo
	retained      map[string][]byte
	changed       chan struct{}
	cancel        context.CancelFunc
	done          chan struct{}
}

type subscriptionInfo struct {
	qos      byte
	callback MessageHandler
}

type Message mqtt.Message
type MessageHandler func(client *Client, message Message)

// NewClient starts connecting in the background so polling can continue through a broker outage.
func NewClient(uri *url.URL, clientID string, prefix string) (*Client, error) {
	client := &Client{
		URI:           uri,
		ClientID:      clientID,
		Prefix:        prefix,
		subscriptions: make(map[string]*subscriptionInfo),
		retained:      make(map[string][]byte),
		changed:       make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	opts, err := createClientOptions(client)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	client.cancel = cancel
	go client.run(ctx, opts)
	return client, nil
}

func (client *Client) Close() {
	client.cancel()
	<-client.done
}

func (client *Client) PublishMany(topic string, values map[string]interface{}) error {
	for key, val := range values {
		if err := client.PublishRaw(client.Prefix+"/"+topic+"/"+key, val); err != nil {
			return err
		}
	}
	return nil
}

func (client *Client) PublishRaw(topic string, val interface{}) error {
	switch payload := val.(type) {
	case string:
		client.retain(topic, []byte(payload))
	case []byte:
		client.retain(topic, bytes.Clone(payload))
	default:
		return client.PublishJSON(topic, val)
	}
	return nil
}

func (client *Client) PublishJSON(topic string, val interface{}) error {
	payload, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal MQTT payload for %s: %w", topic, err)
	}
	client.retain(topic, payload)
	return nil
}

func (client *Client) retain(topic string, payload []byte) {
	client.mu.Lock()
	client.retained[topic] = payload
	client.mu.Unlock()
	client.notifyChanged()
}

// Subscribe registers a subscription to install on this and subsequent connections.
func (client *Client) Subscribe(topic string, qos byte, callback MessageHandler) error {
	if topic == "" || qos > 2 || callback == nil {
		return fmt.Errorf("invalid MQTT subscription for %q", topic)
	}
	client.mu.Lock()
	client.subscriptions[client.Prefix+"/"+topic] = &subscriptionInfo{qos: qos, callback: callback}
	client.mu.Unlock()
	client.notifyChanged()
	return nil
}

func (client *Client) notifyChanged() {
	select {
	case client.changed <- struct{}{}:
	default:
	}
}

func (client *Client) run(ctx context.Context, opts *mqtt.ClientOptions) {
	defer close(client.done)

	retryDelay := time.Second
	for ctx.Err() == nil {
		// Each attempt gets a clean Paho client. Only our latest retained values
		// are replayed, never an old queue of in-flight publishes.
		lost := make(chan error, 1)
		opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			lost <- err
		})
		connection := mqtt.NewClient(opts)
		err := waitForToken(ctx, connection.Connect(), 2*connectTimeout+time.Second)
		if err == nil {
			connectedAt := time.Now()
			err = client.serve(ctx, connection, lost)
			if time.Since(connectedAt) >= maxRetryDelay {
				retryDelay = time.Second
			}
			if ctx.Err() != nil && connection.IsConnectionOpen() {
				offline := connection.Publish(client.Prefix+"/device/status", 1, true, "offline")
				offline.WaitTimeout(time.Second)
			}
			connection.Disconnect(250)
		} else if err == context.DeadlineExceeded || ctx.Err() != nil {
			connection.Disconnect(250)
		}
		if ctx.Err() != nil {
			return
		}

		log.Warnf("MQTT session failed: %v; retrying in %s", err, retryDelay)
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
	for {
		client.mu.Lock()
		subscriptions := maps.Clone(client.subscriptions)
		retained := maps.Clone(client.retained)
		client.mu.Unlock()

		for topic, sub := range subscriptions {
			if subscribed[topic] == sub {
				continue
			}
			token := connection.Subscribe(topic, sub.qos, func(_ mqtt.Client, msg mqtt.Message) {
				sub.callback(client, msg)
			})
			if err := waitForToken(ctx, token, ackTimeout); err != nil {
				return fmt.Errorf("subscribe to %s: %w", topic, err)
			}
			if granted, ok := token.(*mqtt.SubscribeToken).Result()[topic]; !ok || granted > 2 {
				return fmt.Errorf("MQTT broker rejected subscription to %s", topic)
			}
			subscribed[topic] = sub
		}

		if !online {
			token := connection.Publish(client.Prefix+"/device/status", 1, true, "online")
			if err := waitForToken(ctx, token, ackTimeout); err != nil {
				return fmt.Errorf("publish MQTT availability: %w", err)
			}
			log.Info("MQTT connected")
			online = true
		}

		if err := publishRetained(ctx, connection, retained, sent); err != nil {
			return err
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

func createClientOptions(client *Client) (*mqtt.ClientOptions, error) {
	opts := mqtt.NewClientOptions()
	host := client.URI.Hostname()
	if host == "" {
		return nil, fmt.Errorf("MQTT broker hostname is required")
	}

	port := client.URI.Port()
	switch client.URI.Scheme {
	case "mqtts":
		if port == "" {
			port = "8883"
		}
		query := client.URI.Query()
		tlsCert := query.Get("tls_cert")
		tlsKey := query.Get("tls_key")
		caCert := query.Get("tls_cacert")

		tlsConfig := &tls.Config{
			InsecureSkipVerify: query.Get("insecure") == "true",
		}
		if (tlsCert == "") != (tlsKey == "") {
			return nil, fmt.Errorf("MQTT TLS client certificate and key must be supplied together")
		}
		if tlsCert != "" {
			cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
			if err != nil {
				return nil, fmt.Errorf("load MQTT TLS certificate and key: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		if caCert != "" {
			caCertData, err := os.ReadFile(caCert)
			if err != nil {
				return nil, fmt.Errorf("read MQTT CA certificate: %w", err)
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
		return nil, fmt.Errorf("unsupported MQTT URL scheme %q", client.URI.Scheme)
	}

	opts.SetUsername(client.URI.User.Username())
	password, _ := client.URI.User.Password()
	opts.SetPassword(password)
	opts.SetClientID(client.ClientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetDialer(&net.Dialer{Timeout: connectTimeout})
	opts.SetWriteTimeout(ackTimeout)
	opts.SetKeepAlive(15 * time.Second)
	opts.SetPingTimeout(5 * time.Second)
	opts.SetWill(client.Prefix+"/device/status", "offline", 1, true)
	return opts, nil
}
