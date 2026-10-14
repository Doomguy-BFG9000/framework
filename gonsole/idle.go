// SPDX-License-Identifier: Apache-2.0

package gonsole

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// idler counts what is open on one server and ends its run once nothing has been open for its window.
type idler struct {
	mu     sync.Mutex
	open   int
	quiet  time.Time
	window time.Duration
	run    context.Context
	end    context.CancelFunc
	logger *slog.Logger
	timer  *time.Timer
}

// newIdler returns an idler that calls end once nothing has been open for window, unless run already ended.
func newIdler(run context.Context, end context.CancelFunc, window time.Duration, logger *slog.Logger) *idler {
	idle := &idler{quiet: time.Now(), window: window, run: run, end: end, logger: logger}
	idle.timer = time.AfterFunc(window, idle.fire)
	return idle
}

// see counts a connection from its new state until it closes or is hijacked.
func (i *idler) see(_ net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		i.hold()
	case http.StateClosed, http.StateHijacked:
		i.release()
	}
}

// wrap returns next, counting each request while it runs.
func (i *idler) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.hold()
		defer i.release()
		next.ServeHTTP(w, r)
	})
}

// hold counts one more open thing.
func (i *idler) hold() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.open++
}

// release counts one open thing fewer, starting the window again once nothing is open.
func (i *idler) release() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.open--
	if i.open == 0 {
		i.quiet = time.Now()
		i.timer.Reset(i.window)
	}
}

// fire ends the run once nothing has been open for a full window and the run is still live.
func (i *idler) fire() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.due() || i.run.Err() != nil {
		return
	}
	i.logger.Info("no connection for the idle exit", "idle_exit", i.window)
	i.end()
}

// due reports whether nothing is open and nothing has been for a full window.
func (i *idler) due() bool {
	return i.open == 0 && time.Since(i.quiet) >= i.window
}

// stop stops the timer and ends the run.
func (i *idler) stop() {
	i.timer.Stop()
	i.end()
}

// idling returns the run of srv under ctx, ending once nothing has been open for window, zero never, and its stop.
func idling(
	ctx context.Context, srv *http.Server, window time.Duration, logger *slog.Logger,
) (context.Context, func()) {
	if window == 0 {
		return ctx, func() {}
	}
	run, end := context.WithCancel(ctx)
	idle := newIdler(run, end, window, logger)
	srv.ConnState = chained(idle.see, srv.ConnState)
	srv.Handler = idle.wrap(srv.Handler)
	return run, idle.stop
}

// chained returns a hook that calls first and then second, first alone when second is nil.
func chained(first, second func(net.Conn, http.ConnState)) func(net.Conn, http.ConnState) {
	if second == nil {
		return first
	}
	return func(conn net.Conn, state http.ConnState) {
		first(conn, state)
		second(conn, state)
	}
}
