package probe

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// probeTCP checks whether the target host:port accepts a TCP connection.
func probeTCP(ctx context.Context, m store.Monitor, _ time.Duration) outcome {
	address := net.JoinHostPort(m.Host, strconv.FormatUint(uint64(m.Port), 10))

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		stage, msg := classifyStage(err)
		return outcome{status: store.StatusDown, stage: stage, errMsg: msg, port: m.Port}
	}
	remote := conn.RemoteAddr().String()
	_ = conn.Close()

	return outcome{
		status:        store.StatusHealthy,
		stage:         StageNone,
		remoteAddress: remote,
		port:          m.Port,
	}
}
