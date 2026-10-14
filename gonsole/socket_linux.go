// SPDX-License-Identifier: Apache-2.0

//go:build linux

package gonsole

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strings"
	"syscall"
)

// socketMode is the mode of every socket file ServeListening creates.
const socketMode = 0o600

// socketPathLimit is the longest socket path in bytes the kernel binds.
const socketPathLimit = len(syscall.RawSockaddrUnix{}.Path) - 1

// checkSocketPath refuses an abstract socket name and a path longer than the kernel binds.
func checkSocketPath(path string) error {
	if strings.HasPrefix(path, "@") {
		return complaint("must be a file path, not an abstract name", path)
	}
	if len(path) > socketPathLimit {
		return fmt.Errorf("must be at most %d bytes, got %d", socketPathLimit, len(path))
	}
	return nil
}

// listenSocket returns a listener on a new socket at path that only this account can open.
func listenSocket(ctx context.Context, path string) (net.Listener, error) {
	if err := clearStale(ctx, path); err != nil {
		return nil, err
	}
	config := net.ListenConfig{Control: privately}
	listener, err := config.Listen(context.WithoutCancel(ctx), "unix", path)
	if err != nil {
		return nil, err
	}
	if !private(path) {
		_ = listener.Close()
		return nil, fmt.Errorf("the socket at %s is not mode %04o, check the umask", path, socketMode)
	}
	return listener, nil
}

// clearStale removes a socket at path that nothing listens on, refusing whatever else it finds there.
func clearStale(ctx context.Context, path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s is not a socket, so it stays in place", path)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(context.WithoutCancel(ctx), "unix", path)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("a server already listens on %s", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("check the socket at %s: %w", path, err)
	}
	return os.Remove(path)
}

// privately sets the mode of the socket c holds to socketMode before it binds.
func privately(_, _ string, c syscall.RawConn) error {
	var fchmod error
	cerr := c.Control(func(fd uintptr) { fchmod = syscall.Fchmod(int(fd), socketMode) })
	return errors.Join(cerr, fchmod)
}

// private reports whether path is a socket of mode socketMode.
func private(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode() == fs.ModeSocket|socketMode
}
