package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// EventBus is a simple pub/sub bus for SSE events.
type EventBus struct {
	subscribers map[chan SSEEvent]struct{}
	subscribe   chan chan SSEEvent
	unsubscribe chan chan SSEEvent
	publish     chan SSEEvent
	quit        chan struct{}
}

// SSEEvent is a server-sent event payload.
type SSEEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NewEventBus creates and starts an EventBus.
func NewEventBus() *EventBus {
	eb := &EventBus{
		subscribers: make(map[chan SSEEvent]struct{}),
		subscribe:   make(chan chan SSEEvent, 64),
		unsubscribe: make(chan chan SSEEvent, 64),
		publish:     make(chan SSEEvent, 256),
		quit:        make(chan struct{}),
	}
	go eb.run()
	return eb
}

// Publish sends an event to all active subscribers.
func (eb *EventBus) Publish(event SSEEvent) {
	select {
	case eb.publish <- event:
	default:
		// Drop if bus is full to avoid blocking callers.
	}
}

// Subscribe returns a channel that receives published events.
func (eb *EventBus) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	eb.subscribe <- ch
	return ch
}

// Unsubscribe removes the channel from the subscriber list.
func (eb *EventBus) Unsubscribe(ch chan SSEEvent) {
	eb.unsubscribe <- ch
}

// Stop shuts down the event bus.
func (eb *EventBus) Stop() {
	close(eb.quit)
}

func (eb *EventBus) run() {
	for {
		select {
		case ch := <-eb.subscribe:
			eb.subscribers[ch] = struct{}{}
		case ch := <-eb.unsubscribe:
			delete(eb.subscribers, ch)
			close(ch)
		case ev := <-eb.publish:
			for ch := range eb.subscribers {
				select {
				case ch <- ev:
				default:
					// Slow subscriber — drop event rather than block.
				}
			}
		case <-eb.quit:
			return
		}
	}
}

// EventsHandler handles SSE connections.
type EventsHandler struct {
	bus    *EventBus
	logger *zap.Logger
}

// NewEventsHandler creates a new EventsHandler.
func NewEventsHandler(bus *EventBus, logger *zap.Logger) *EventsHandler {
	return &EventsHandler{bus: bus, logger: logger}
}

// Stream upgrades the connection to SSE and streams events to the client.
// GET /api/v1/events
func (h *EventsHandler) Stream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ch := h.bus.Subscribe()
	defer h.bus.Unsubscribe(ch)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			data, err := json.Marshal(ev)
			if err != nil {
				h.logger.Error("marshal sse event", zap.Error(err))
				return true
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true

		case <-ticker.C:
			// Keepalive ping to prevent proxies from closing idle connections.
			fmt.Fprintf(w, ": ping\n\n")
			return true

		case <-c.Request.Context().Done():
			return false
		}
	})
}
