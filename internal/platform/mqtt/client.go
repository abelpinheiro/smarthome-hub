// Package mqtt encapsulates the hub's MQTT v5 client.
//
// Decision (ADR-0002): MQTT is the device-facing protocol. It gives us QoS,
// Last Will & Testament (automatic offline detection) and behaves well on
// unreliable networks — all with a minimal RAM and battery footprint on the
// firmware side.
//
// Topic hierarchy (public contract — changing it breaks deployed firmware):
//
//	home/{homeID}/dev/{deviceID}/telemetry   device -> hub   (QoS 0/1)
//	home/{homeID}/dev/{deviceID}/state       device -> hub   (retained)
//	home/{homeID}/dev/{deviceID}/cmd         hub -> device   (QoS 1)
//	home/{homeID}/dev/{deviceID}/cmd/ack     device -> hub   (QoS 1)
//
// Putting {deviceID} in the topic is not cosmetic: it is what enables
// per-device ACLs on the broker, so a compromised device can only publish
// under its own path (AppSec, principle of least privilege).
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

// Options holds the client connection parameters.
type Options struct {
	URL       string
	ClientID  string
	Username  string
	Password  string
	KeepAlive time.Duration
	Logger    *slog.Logger
}

// MessageHandler processes a message received from the broker.
type MessageHandler func(ctx context.Context, topic string, payload []byte)

// Client wraps autopaho with automatic reconnection.
//
// Why autopaho and not bare paho: the hub runs in a house, with flaky Wi-Fi and
// power cuts. Reconnection with backoff is a requirement, not a nice-to-have,
// and hand-rolling it means reinventing a well-tested wheel.
type Client struct {
	cm        *autopaho.ConnectionManager
	connected atomic.Bool
	log       *slog.Logger
}

// Connect establishes the managed connection to the broker.
//
// The given ctx governs the ENTIRE connection lifecycle: cancelling it shuts
// the manager down and stops reconnection attempts. Pass the application
// context, never a request context.
func Connect(ctx context.Context, opts Options, onMessage MessageHandler) (*Client, error) {
	u, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid broker URL: %w", err)
	}

	c := &Client{log: opts.Logger}

	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     uint16(opts.KeepAlive.Seconds()),
		CleanStartOnInitialConnection: false,
		// The session survives short disconnects so we do not lose
		// subscriptions on every network hiccup.
		SessionExpiryInterval: 60,
		ConnectUsername:       opts.Username,
		ConnectPassword:       []byte(opts.Password),

		OnConnectionUp: func(_ *autopaho.ConnectionManager, _ *paho.Connack) {
			c.connected.Store(true)
			c.log.Info("connected to MQTT broker", slog.String("url", u.Redacted()))
		},
		OnConnectError: func(err error) {
			c.connected.Store(false)
			c.log.Warn("failed to connect to MQTT broker", slog.String("error", err.Error()))
		},

		ClientConfig: paho.ClientConfig{
			ClientID: opts.ClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) {
					if onMessage != nil {
						onMessage(ctx, pr.Packet.Topic, pr.Packet.Payload)
					}
					return true, nil
				},
			},
			OnClientError: func(err error) {
				c.connected.Store(false)
				c.log.Warn("MQTT client error", slog.String("error", err.Error()))
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				c.connected.Store(false)
				c.log.Warn("disconnected by broker",
					slog.Int("reason_code", int(d.ReasonCode)))
			},
		},
	}

	cm, err := autopaho.NewConnection(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create MQTT connection manager: %w", err)
	}
	c.cm = cm
	return c, nil
}

// AwaitConnection blocks until the first connection is established or the
// context expires.
func (c *Client) AwaitConnection(ctx context.Context) error {
	if err := c.cm.AwaitConnection(ctx); err != nil {
		return fmt.Errorf("await broker connection: %w", err)
	}
	return nil
}

// Subscribe subscribes to a topic (or wildcard pattern) on the broker.
func (c *Client) Subscribe(ctx context.Context, topic string, qos byte) error {
	_, err := c.cm.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: topic, QoS: qos},
		},
	})
	if err != nil {
		return fmt.Errorf("subscribe to topic %q: %w", topic, err)
	}
	c.log.Info("MQTT subscription registered",
		slog.String("topic", topic), slog.Int("qos", int(qos)))
	return nil
}

// Publish sends a message to the broker.
//
// Use is deliberately restricted: only core/command may publish commands, so
// that arbitration, idempotency and auditing (ADR-0006) cannot be bypassed by
// any module.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte, qos byte, retain bool) error {
	_, err := c.cm.Publish(ctx, &paho.Publish{
		Topic:   topic,
		QoS:     qos,
		Retain:  retain,
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("publish to %q: %w", topic, err)
	}
	return nil
}

// IsConnected reports the current connection state. Used by /health.
func (c *Client) IsConnected() bool { return c.connected.Load() }

// Close gracefully terminates the connection.
func (c *Client) Close(ctx context.Context) error {
	if c.cm == nil {
		return nil
	}
	return c.cm.Disconnect(ctx)
}
