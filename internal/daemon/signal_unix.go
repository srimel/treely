//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyShutdown() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	return ch, func() { signal.Stop(ch) }
}
