// SPDX-License-Identifier: Apache-2.0

package gonsole

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestVisitorAddrReadsOneAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []string
		want   string
		ok     bool
	}{
		{"an IPv4 address", []string{"203.0.113.7"}, "203.0.113.7:0", true},
		{"an IPv4 address with its port", []string{"203.0.113.7:8443"}, "203.0.113.7:8443", true},
		{"an IPv6 address", []string{"2001:db8::1"}, "[2001:db8::1]:0", true},
		{"an IPv6 address with its port", []string{"[2001:db8::1]:443"}, "[2001:db8::1]:443", true},
		{"an IPv4-mapped address", []string{"::ffff:203.0.113.7"}, "203.0.113.7:0", true},
		{"an address with a zone", []string{"fe80::1%eth0"}, "[fe80::1]:0", true},
		{"no value", nil, "", false},
		{"an empty value", []string{""}, "", false},
		{"two values", []string{"203.0.113.7", "198.51.100.2"}, "", false},
		{"a comma list", []string{"203.0.113.7, 198.51.100.2"}, "", false},
		{"a comma list after a zone", []string{"fe80::1%eth0, 203.0.113.7"}, "", false},
		{"a comma list after a zone with a port", []string{"[fe80::1%eth0,203.0.113.7]:443"}, "", false},
		{"a word", []string{"unknown"}, "", false},
		{"a port above the highest", []string{"203.0.113.7:99999"}, "", false},
		{"an IPv6 address in brackets with no port", []string{"[2001:db8::1]"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := visitorAddr(tc.values)

			if got != tc.want || ok != tc.ok {
				t.Errorf("visitorAddr(%q) = %q, %t, want %q, %t", tc.values, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// idleWindow is the idle exit the idler tests run under, in the fake time of a synctest bubble.
const idleWindow = 10 * time.Second

// tick is the shortest step of fake time.
const tick = time.Nanosecond

// ledger is a log target that keeps every line, safe to read while another goroutine writes.
type ledger struct {
	mu   sync.Mutex
	text strings.Builder
}

// Write keeps p.
func (l *ledger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

// String returns everything kept.
func (l *ledger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

// idleRun is an idler under test with the run it ends, how often it ended it and what it logged.
type idleRun struct {
	idle   *idler
	run    context.Context
	ends   *atomic.Int32
	logged *ledger
}

// startIdle returns an idler over a run of parent that ends once nothing has been open for idleWindow.
func startIdle(parent context.Context) idleRun {
	run, end := context.WithCancel(parent)
	ends := new(atomic.Int32)
	logged := new(ledger)
	counted := func() {
		ends.Add(1)
		end()
	}
	idle := newIdler(run, counted, idleWindow, slog.New(slog.NewTextHandler(logged, nil)))
	return idleRun{idle: idle, run: run, ends: ends, logged: logged}
}

// saidIdle reports whether the idler logged its idle line.
func (r idleRun) saidIdle() bool {
	return strings.Contains(r.logged.String(), `level=INFO msg="no connection for the idle exit" idle_exit=10s`)
}

// liveAfter lets d of fake time pass and reports whether run is still live.
func liveAfter(run context.Context, d time.Duration) bool {
	time.Sleep(d)
	synctest.Wait()
	return run.Err() == nil
}

// endsAWindowAfter fails t unless the run, from now, is live a tick before a window passes and ended once it has.
func (r idleRun) endsAWindowAfter(t *testing.T, event string) {
	t.Helper()
	closing := liveAfter(r.run, idleWindow-tick)
	closed := liveAfter(r.run, tick)
	if !closing {
		t.Errorf("the run ended a tick before a window passed since %s", event)
	}
	if closed {
		t.Errorf("the run is live a window after %s, want it ended", event)
	}
}

func TestIdlerEndsTheRunAtTheWindow(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()

		early := liveAfter(r.run, idleWindow-tick)
		late := liveAfter(r.run, tick)

		if !early {
			t.Errorf("the run ended a tick before the window passed")
		}
		if late {
			t.Errorf("the run is live once the window passed with nothing open, want it ended")
		}
		if !r.saidIdle() {
			t.Errorf("log = %q, want the idle line", r.logged.String())
		}
	})
}

func TestIdlerWaitsWhileAConnectionIsOpen(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()

		r.idle.see(nil, http.StateNew)
		open := liveAfter(r.run, 2*idleWindow)
		r.idle.see(nil, http.StateClosed)

		if !open {
			t.Errorf("the run ended while a connection was open")
		}
		r.endsAWindowAfter(t, "the close")
	})
}

func TestIdlerStartsTheWindowAgainAfterAQuickConnection(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()
		time.Sleep(idleWindow / 2)

		r.idle.see(nil, http.StateNew)
		r.idle.see(nil, http.StateClosed)
		first := liveAfter(r.run, idleWindow/2)
		closing := liveAfter(r.run, idleWindow/2-tick)
		closed := liveAfter(r.run, tick)

		if !first {
			t.Errorf("the run ended when the first window passed, want the window started again at the close")
		}
		if !closing {
			t.Errorf("the run ended a tick before a window passed since the close")
		}
		if closed {
			t.Errorf("the run is live a window after the close, want it ended")
		}
	})
}

func TestIdlerWaitsAWindowAfterAStaleFire(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()
		r.idle.see(nil, http.StateNew)
		time.Sleep(idleWindow)
		synctest.Wait()

		r.idle.see(nil, http.StateClosed)
		r.idle.fire()

		if r.run.Err() != nil {
			t.Errorf("the run ended on a timer fire right after the close, want it to wait a window from the close")
		}
		r.endsAWindowAfter(t, "the close")
	})
}

func TestIdlerWaitsForARunningHandler(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()
		release := make(chan struct{})
		handler := r.idle.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))

		r.idle.see(nil, http.StateNew)
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		synctest.Wait()
		r.idle.see(nil, http.StateHijacked)
		running := liveAfter(r.run, 2*idleWindow)
		close(release)

		if !running {
			t.Errorf("the run ended while the handler of a hijacked connection ran")
		}
		r.endsAWindowAfter(t, "the handler returned")
	})
}

// recovered serves one request through handler and returns what it panicked with.
func recovered(handler http.Handler) (panicked any) {
	defer func() { panicked = recover() }()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return nil
}

func TestIdlerReleasesAHandlerThatPanics(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := startIdle(t.Context())
		defer r.idle.stop()
		handler := r.idle.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
		time.Sleep(idleWindow / 2)

		panicked := recovered(handler)

		if panicked != http.ErrAbortHandler {
			t.Errorf("the wrapped handler panicked with %v, want the handler's own panic passed on", panicked)
		}
		r.endsAWindowAfter(t, "the handler panicked")
	})
}

func TestIdlerStaysQuietAfterTheRunEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		parent, endParent := context.WithCancel(t.Context())
		r := startIdle(parent)
		defer r.idle.stop()

		endParent()
		time.Sleep(idleWindow)
		synctest.Wait()

		if r.saidIdle() {
			t.Errorf("log = %q, want no idle line once the run ended", r.logged.String())
		}
		if ends := r.ends.Load(); ends != 0 {
			t.Errorf("the idler ended the run %d times after it ended, want none", ends)
		}
	})
}

func TestIdlingKeepsTheServersOwnConnState(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var seen []http.ConnState
		srv := &http.Server{ConnState: func(_ net.Conn, state http.ConnState) { seen = append(seen, state) }}
		run, stop := idling(t.Context(), srv, idleWindow, slog.New(slog.DiscardHandler))
		defer stop()
		opening := []http.ConnState{http.StateNew, http.StateActive, http.StateNew, http.StateIdle, http.StateHijacked}

		for _, state := range opening {
			srv.ConnState(nil, state)
		}
		open := liveAfter(run, 2*idleWindow)
		srv.ConnState(nil, http.StateClosed)
		closed := liveAfter(run, idleWindow)

		if want := append(opening, http.StateClosed); !slices.Equal(seen, want) {
			t.Errorf("the server's own hook saw %v, want %v", seen, want)
		}
		if !open {
			t.Errorf("the run ended while a connection of the server was open, want the idler to see it")
		}
		if closed {
			t.Errorf("the run is live a window after the last connection closed, want it ended")
		}
	})
}

func TestIdlingCountsTheServersRunningRequests(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release })}
		run, stop := idling(t.Context(), srv, idleWindow, slog.New(slog.DiscardHandler))
		defer stop()

		go srv.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		running := liveAfter(run, 2*idleWindow)
		close(release)
		returned := liveAfter(run, idleWindow)

		if !running {
			t.Errorf("the run ended while a request of the server ran, want the idler to count it")
		}
		if returned {
			t.Errorf("the run is live a window after the request returned, want it ended")
		}
	})
}

func TestServeListeningStopsItsIdlerWhenServingFails(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		timeouts := Timeouts{ReadHeader: time.Second, Read: time.Second, Idle: time.Minute, Grace: time.Second,
			CancelGrace: time.Second, StopGrace: time.Second}
		srv := NewServer("", http.NotFoundHandler(), timeouts)
		_ = srv.Close()
		logged := new(ledger)
		at := Listening{Address: "127.0.0.1:0", IdleExit: idleWindow}

		err := ServeListening(t.Context(), srv, timeouts, at, nil, slog.New(slog.NewTextHandler(logged, nil)))
		time.Sleep(idleWindow)
		synctest.Wait()

		if !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("ServeListening() = %v, want the serving failure", err)
		}
		if strings.Contains(logged.String(), "no connection for the idle exit") {
			t.Errorf("log = %q, want no idle line once ServeListening returned", logged.String())
		}
	})
}

// still is a handler that answers nothing, comparable with ==.
type still struct{}

// ServeHTTP answers nothing.
func (still) ServeHTTP(http.ResponseWriter, *http.Request) {}

func TestIdlingWithZeroLeavesTheServerAlone(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		own := func(net.Conn, http.ConnState) {}
		srv := &http.Server{ConnState: own, Handler: still{}}

		run, stop := idling(t.Context(), srv, 0, slog.New(slog.DiscardHandler))
		stop()

		if run != t.Context() {
			t.Errorf("idling() returned a new context, want the one it was given")
		}
		if reflect.ValueOf(srv.ConnState).Pointer() != reflect.ValueOf(own).Pointer() {
			t.Errorf("ConnState changed, want the server's own hook left in place")
		}
		if srv.Handler != http.Handler(still{}) {
			t.Errorf("Handler = %T, want the server's own handler left in place", srv.Handler)
		}
	})
}
