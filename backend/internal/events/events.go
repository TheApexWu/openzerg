// Package events provides a tiny in-process pub/sub broker for the live
// evolution stream that the HTTP+SSE server in internal/api consumes.
//
// Design notes:
//
//   - Bounded ring buffer (RingCapacity events) so late-connecting SSE
//     clients can replay recent context via Last-Event-ID without forcing
//     the whole run history into memory.
//   - Monotonic Seq counter — clients use it as the SSE event id.
//   - Subscribe returns a buffered channel; a slow subscriber gets dropped
//     events rather than back-pressuring the publisher (we never block
//     the evolution loop on a stuck UI).
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is the envelope every SSE message carries. Payload is opaque JSON
// so packages above can publish typed structs without the broker caring
// about their shape.
type Event struct {
	Type    string          `json:"type"`
	RunID   string          `json:"run_id,omitempty"`
	Seq     int64           `json:"seq"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RingCapacity bounds the replay buffer. Two thousand events comfortably
// covers a full 4-generation run (≤15 pods/gen, a handful of events each).
const RingCapacity = 2000

// subscriberBufferSize is the per-subscriber channel buffer. Picked large
// enough to absorb a generation's burst without starving the broker.
const subscriberBufferSize = 64

// Broker fans Publish calls out to every active Subscribe channel. It also
// retains the last RingCapacity events for replay.
type Broker struct {
	mu          sync.Mutex
	subscribers map[int]chan Event
	nextSubID   int
	nextSeq     int64
	ring        []Event
	ringStart   int
	ringLen     int
}

// NewBroker constructs an empty broker. Cheap; one per openzerg process.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[int]chan Event),
		ring:        make([]Event, RingCapacity),
	}
}

// Publish stamps an event with the next sequence number and current time,
// records it in the ring, and fans it out to all current subscribers. A
// subscriber whose channel is full drops the event rather than blocking
// the publisher.
func (broker *Broker) Publish(eventType, runID string, payload any) Event {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		payloadBytes = []byte(`{}`)
	}
	broker.mu.Lock()
	broker.nextSeq++
	evt := Event{
		Type:    eventType,
		RunID:   runID,
		Seq:     broker.nextSeq,
		TS:      time.Now().UTC(),
		Payload: payloadBytes,
	}
	broker.appendToRingLocked(evt)
	subscriberChannels := make([]chan Event, 0, len(broker.subscribers))
	for _, ch := range broker.subscribers {
		subscriberChannels = append(subscriberChannels, ch)
	}
	broker.mu.Unlock()

	for _, ch := range subscriberChannels {
		select {
		case ch <- evt:
		default:
			// Subscriber is slow; drop. Their next reconnect with
			// Last-Event-ID picks up from the ring.
		}
	}
	return evt
}

// Subscribe registers a new subscriber and returns its channel plus an
// Unsubscribe closer the caller must invoke when done. If sinceSeq > 0,
// any events still in the ring with Seq > sinceSeq are delivered immediately
// (in order) before any live event. The replay is enqueued on the
// subscriber's channel under the broker lock so live publishes that arrive
// concurrently always land after the replay snapshot.
func (broker *Broker) Subscribe(sinceSeq int64) (<-chan Event, func()) {
	bufferSize := subscriberBufferSize
	broker.mu.Lock()
	replay := broker.replayFromLocked(sinceSeq)
	if len(replay) > bufferSize {
		bufferSize = len(replay) + subscriberBufferSize
	}
	subscriberChannel := make(chan Event, bufferSize)
	for _, evt := range replay {
		subscriberChannel <- evt
	}
	subscriberID := broker.nextSubID
	broker.nextSubID++
	broker.subscribers[subscriberID] = subscriberChannel
	broker.mu.Unlock()

	unsubscribe := func() {
		broker.mu.Lock()
		if existing, ok := broker.subscribers[subscriberID]; ok {
			delete(broker.subscribers, subscriberID)
			close(existing)
		}
		broker.mu.Unlock()
	}
	return subscriberChannel, unsubscribe
}

// Recent returns the most recent up-to-n events in chronological order.
// Useful for an HTTP GET that wants a snapshot without opening SSE.
func (broker *Broker) Recent(n int) []Event {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if n <= 0 || broker.ringLen == 0 {
		return nil
	}
	if n > broker.ringLen {
		n = broker.ringLen
	}
	out := make([]Event, 0, n)
	startOffset := broker.ringLen - n
	for i := 0; i < n; i++ {
		idx := (broker.ringStart + startOffset + i) % len(broker.ring)
		out = append(out, broker.ring[idx])
	}
	return out
}

func (broker *Broker) appendToRingLocked(evt Event) {
	if broker.ringLen < len(broker.ring) {
		idx := (broker.ringStart + broker.ringLen) % len(broker.ring)
		broker.ring[idx] = evt
		broker.ringLen++
		return
	}
	broker.ring[broker.ringStart] = evt
	broker.ringStart = (broker.ringStart + 1) % len(broker.ring)
}

func (broker *Broker) replayFromLocked(sinceSeq int64) []Event {
	if broker.ringLen == 0 {
		return nil
	}
	out := make([]Event, 0, broker.ringLen)
	for i := 0; i < broker.ringLen; i++ {
		idx := (broker.ringStart + i) % len(broker.ring)
		evt := broker.ring[idx]
		if evt.Seq > sinceSeq {
			out = append(out, evt)
		}
	}
	return out
}
