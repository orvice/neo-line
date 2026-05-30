package probe

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// probeTCP checks whether the target host:port accepts a TCP connection.
func probeTCP(ctx context.Context, m store.Monitor, _ time.Duration, logger *slog.Logger) outcome {
	address := net.JoinHostPort(m.Host, strconv.FormatUint(uint64(m.Port), 10))
	logger.Debug("tcp dial", "address", address)

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		stage, msg := classifyStage(err)
		logger.Debug("tcp dial failed", "address", address, "stage", stage, "error", msg)
		return outcome{status: store.StatusDown, stage: stage, errMsg: msg, port: m.Port}
	}
	remote := conn.RemoteAddr().String()
	_ = conn.Close()
	logger.Debug("tcp connection established", "remote_address", remote)

	return outcome{
		status:        store.StatusHealthy,
		stage:         StageNone,
		remoteAddress: remote,
		port:          m.Port,
	}
}
