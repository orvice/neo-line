package certmanager

import (
	"context"

	"github.com/orvice/neo-line/internal/metric"
)

func (m *Manager) refreshValidityMetrics(ctx context.Context) {
	const pageSize int64 = 500
	counts := map[string]int{}
	now := m.clock.Now()
	token := ""
	for {
		certs, next, err := m.store.ListManagedCertificates(ctx, pageSize, token)
		if err != nil {
			return
		}
		for _, cert := range certs {
			validity, _ := computeValidity(cert, now)
			counts[validity]++
		}
		if next == "" {
			break
		}
		token = next
	}
	metric.RefreshManagedCertValidity(counts)
}
