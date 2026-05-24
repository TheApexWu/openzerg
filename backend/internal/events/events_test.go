package events

import (
	"testing"
)

func TestBrokerPublishSubscribeAndReplay(t *testing.T) {
	broker := NewBroker()

	// publish before any subscriber — should still record in the ring
	broker.Publish("run_start", "r1", map[string]any{"a": 1})
	broker.Publish("generation_start", "r1", map[string]any{"gen": 1})

	subscriberChannel, unsubscribe := broker.Subscribe(0)
	defer unsubscribe()

	got := make([]Event, 0, 4)
	// replay (2) + 1 live event
	broker.Publish("pod_spawn", "r1", map[string]any{"pod": "p0"})

	for len(got) < 3 {
		select {
		case evt := <-subscriberChannel:
			got = append(got, evt)
		}
	}

	if got[0].Seq != 1 || got[1].Seq != 2 || got[2].Seq != 3 {
		t.Fatalf("unexpected seq order: %d %d %d", got[0].Seq, got[1].Seq, got[2].Seq)
	}
	if got[2].Type != "pod_spawn" {
		t.Fatalf("expected pod_spawn live event, got %q", got[2].Type)
	}
}

func TestBrokerRingWrapsAndDropsOldest(t *testing.T) {
	broker := NewBroker()
	// override ring to a small size for the test by re-allocating
	broker.ring = make([]Event, 3)

	broker.Publish("a", "", nil)
	broker.Publish("b", "", nil)
	broker.Publish("c", "", nil)
	broker.Publish("d", "", nil) // evicts "a"

	recent := broker.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}
	if recent[0].Type != "b" || recent[1].Type != "c" || recent[2].Type != "d" {
		t.Fatalf("unexpected ring contents: %v", recent)
	}
}

func TestBrokerSubscribeSinceSeqSkipsOldEvents(t *testing.T) {
	broker := NewBroker()
	broker.Publish("a", "", nil) // seq 1
	broker.Publish("b", "", nil) // seq 2
	broker.Publish("c", "", nil) // seq 3

	subscriberChannel, unsubscribe := broker.Subscribe(2)
	defer unsubscribe()

	evt := <-subscriberChannel
	if evt.Seq != 3 || evt.Type != "c" {
		t.Fatalf("expected replay to skip up to seq 2; got seq=%d type=%q", evt.Seq, evt.Type)
	}
}
