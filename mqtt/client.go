/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021 Mark Lipscombe.
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
	"encoding/json"
	"fmt"
	"net/url"
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
	err := client.connect(opts)
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
		jsonVal, err := json.Marshal(val)
		if err != nil {
			log.Errorf("Error marshalling %s: %v", key, val)
			return err
		}
		token := client.connection.Publish(fmt.Sprintf("%s/%s/%s", client.Prefix, topic, key), 0, true, jsonVal)
		go func() {
			<-token.Done()
			if token.Error() != nil {
				log.Error(token.Error())
			}
		}()
	}
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
	opts.AddBroker(fmt.Sprintf("tcp://%s", uri.Host))
	opts.SetUsername(uri.User.Username())
	password, _ := uri.User.Password()
	opts.SetPassword(password)
	opts.SetClientID(clientId)
	opts.SetAutoReconnect(true)
	return opts
}
