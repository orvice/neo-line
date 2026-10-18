package notify

import (
	"sync"

	"github.com/orvice/neo-line/internal/store"
)

// RecordingSender captures every delivery for tests without HTTP.
type RecordingSender struct {
	mu   sync.Mutex
	Sent []Delivery
}

func (r *RecordingSender) Send(_ store.AlertChannel, d Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := Delivery{
		HumanText: d.HumanText,
	}
	if len(d.WebhookJSON) > 0 {
		copied.WebhookJSON = append([]byte(nil), d.WebhookJSON...)
	}
	r.Sent = append(r.Sent, copied)
	return nil
}

// Deliveries returns a snapshot of captured deliveries.
func (r *RecordingSender) Deliveries() []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Delivery, len(r.Sent))
	for i, d := range r.Sent {
		out[i] = Delivery{HumanText: d.HumanText}
		if len(d.WebhookJSON) > 0 {
			out[i].WebhookJSON = append([]byte(nil), d.WebhookJSON...)
		}
	}
	return out
}
