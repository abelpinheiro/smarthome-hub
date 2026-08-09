// Package bus defines the event bus abstraction between bounded contexts.
//
// Architectural decision (ADR-0001): modules NEVER import each other. All
// inter-module communication goes through this bus. The implementation is
// in-process today; when a context needs to scale on its own, we swap the
// transport for NATS/Kafka without touching a single line of domain code.
//
// That is why Publish takes a context.Context even though it does not need one
// today: the signature is already the signature of a remote transport.
package bus

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// Event is any domain event published on the bus.
// Concrete implementations live in internal/contracts.
type Event interface {
	// EventName is the stable identifier used for routing. Stable means
	// changing it breaks subscribers, so treat it as a public contract.
	EventName() string
}

// Handler processes an event. A returned error is logged but does not stop the
// remaining subscribers: a faulty module must not take the others down. Once
// the transport becomes remote, this is where retry/DLQ hooks in.
type Handler func(ctx context.Context, e Event) error

// Bus is the publish/subscribe contract.
type Bus interface {
	Publish(ctx context.Context, e Event) error
	Subscribe(eventName string, h Handler)
}

// InProcess is the synchronous in-process implementation of the bus.
//
// Deliberate limitation (documented in ADR-0001): delivery is synchronous and
// in-process. There is no durability — if the hub crashes between Publish and
// the handler, the event is lost. Acceptable for telemetry; NOT acceptable for
// commands, which get their own durable path in SH-004.
type InProcess struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	log      *slog.Logger
}

// NewInProcess creates a ready-to-use in-process bus.
func NewInProcess(log *slog.Logger) *InProcess {
	return &InProcess{
		handlers: make(map[string][]Handler),
		log:      log,
	}
}

// Subscribe registers a handler for an event name.
// It must be called during bootstrap, before any Publish.
func (b *InProcess) Subscribe(eventName string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], h)
}

// Publish delivers the event to every registered subscriber.
func (b *InProcess) Publish(ctx context.Context, e Event) error {
	b.mu.RLock()
	hs := b.handlers[e.EventName()]
	b.mu.RUnlock()

	for _, h := range hs {
		if err := h(ctx, e); err != nil {
			// Failure isolation: one broken subscriber must not block the rest.
			b.log.ErrorContext(ctx, "event handler failed",
				slog.String("event", e.EventName()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

// Compile-time check that InProcess satisfies Bus.
var _ Bus = (*InProcess)(nil)

// ErrNoHandler is returned by implementations that require at least one
// subscriber for an event.
var ErrNoHandler = errors.New("no handler registered for event")
