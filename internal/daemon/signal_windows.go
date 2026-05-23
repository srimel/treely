//go:build windows

package daemon

import (
	"os"
	"os/signal"
)

func notifyShutdown() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return ch, func() { signal.Stop(ch) }
}
