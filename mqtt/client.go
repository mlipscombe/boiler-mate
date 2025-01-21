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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	URI        *url.URL
	ClientID   string
	Prefix     string
	connection mqtt.Client
}

type Message mqtt.Message

type MessageHandler func(client *Client, message Message)

func NewClient(uri *url.URL, client_id string, prefix string) (*Client, error) {
	client := Client{
		URI:      uri,
		ClientID: client_id,
		Prefix:   prefix,
	}
	opts := createClientOptions(client.URI, client.ClientID)

	opts.SetWill(fmt.Sprintf("%s/device/status", client.Prefix), "offline", 1, true)
	err := client.connect(opts)

	client.connection.Publish(fmt.Sprintf("%s/device/status", client.Prefix), 1, true, "online")

	return &client, err
}

func (client *Client) connect(opts *mqtt.ClientOptions) error {
	client.connection = mqtt.NewClient(opts)
	token := client.connection.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

func (client *Client) PublishMany(topic string, values map[string]interface{}) error {
	for key, val := range values {
		err := client.PublishRaw(fmt.Sprintf("%s/%s/%s", client.Prefix, topic, key), val)
		if err != nil {
			return err
		}
	}
	return nil
}

func (client *Client) PublishRaw(topic string, val interface{}) error {
	var payload []byte
	switch p := val.(type) {
	case string:
		payload = []byte(p)
	case []byte:
		payload = p
	default:
		jsonVal, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshalling %s: %v", topic, val)
		}
		payload = jsonVal
	}

	token := client.connection.Publish(topic, 0, true, payload)
	go func() {
		<-token.Done()
		if token.Error() != nil {
			log.Error(token.Error())
		}
	}()

	return nil
}

func (client *Client) PublishJSON(topic string, val interface{}) error {
	jsonVal, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshalling %s: %v", topic, val)
	}
	token := client.connection.Publish(topic, 0, true, jsonVal)
	go func() {
		<-token.Done()
		if token.Error() != nil {
			log.Error(token.Error())
		}
	}()

	return nil
}

func (client *Client) Subscribe(topic string, qos byte, callback MessageHandler) error {
	full_topic := fmt.Sprintf("%s/%s", client.Prefix, topic)
	token := client.connection.Subscribe(full_topic, qos, func(_ mqtt.Client, msg mqtt.Message) {
		callback(client, msg)
	})
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

func createClientOptions(uri *url.URL, clientId string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()

	port := uri.Port()
	if port == "" {
		if uri.Scheme == "mqtts" {
			port = "8883"
		} else {
			port = "1883"
		}
	}

	if uri.Scheme == "mqtts" {
		query := uri.Query()
		tlsCert := query.Get("tls_cert")
		tlsKey := query.Get("tls_key")
		caCert := query.Get("tls_cacert")
		insecure := query.Get("insecure")

		tlsConfig := &tls.Config{}

		if insecure == "true" {
			tlsConfig.InsecureSkipVerify = true
		}

		if tlsCert != "" && tlsKey != "" {
			cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
			if err != nil {
				log.Fatalf("failed to load tls cert and key: %v", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		if caCert != "" {
			caCertPool := x509.NewCertPool()
			caCertData, err := os.ReadFile(caCert)
			if err != nil {
				log.Fatalf("failed to read ca cert: %v", err)
			}
			caCertPool.AppendCertsFromPEM(caCertData)
			tlsConfig.RootCAs = caCertPool
		}

		opts.SetTLSConfig(tlsConfig)
		opts.AddBroker(fmt.Sprintf("ssl://%s:%s", uri.Hostname(), port))
	} else {
		opts.AddBroker(fmt.Sprintf("tcp://%s:%s", uri.Hostname(), port))
	}

	opts.SetUsername(uri.User.Username())
	password, _ := uri.User.Password()
	opts.SetPassword(password)
	opts.SetClientID(clientId)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetMaxReconnectInterval(10 * time.Second)
	opts.SetAutoReconnect(true)

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Errorf("mqtt connection lost: %v", err)
	})
	opts.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
		log.Warn("mqtt reconnecting")
	})

	return opts
}
