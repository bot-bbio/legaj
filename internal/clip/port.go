package clip

import (
	"context"
	"math/rand/v2"
	"net"
	"strconv"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// The clip server listens on one port, chosen at first run and kept, and
// the bookmarklet has that port built in (R18). There is no runtime
// fallback: the popup can't tell a closed port from a busy one, so a
// bookmarklet only ever tries its own port.
const (
	// PortKey is the app_meta key holding the server's port.
	PortKey = "clip_port"
	// BookmarkletPortKey holds the port the installed bookmarklet is taken
	// to have: the one a clip last arrived on, or the port LeGaJ had to
	// leave (MarkInstalled).
	BookmarkletPortKey = "clip_bookmarklet_port"
	// PortLow and PortHigh bound the ports LeGaJ picks from: registered
	// ports few programs use, below Windows' dynamic range (49152+).
	PortLow  = 47100
	PortHigh = 47999
	// portTries is how many random ports are tried before giving up.
	portTries = 50
)

// Meta reads and writes small app state (store.DB implements it).
type Meta interface {
	GetMeta(ctx context.Context, key string) (string, bool, error)
	SetMeta(ctx context.Context, key, value string) error
}

// PortStatus is the clip server's startup state.
type PortStatus struct {
	// Port is the port the server listens on.
	Port int `json:"port"`
	// Previous is the persisted port another program held at startup; 0
	// when the port didn't change.
	Previous int `json:"previous"`
	// Bookmarklet is the port the installed bookmarklet is taken to have; 0
	// before the first clip.
	Bookmarklet int `json:"bookmarklet"`
}

// NeedsReinstall reports whether the installed bookmarklet points at
// another port, so the app must show the banner and a regenerated
// bookmarklet (R18). It stays true across restarts until a clip arrives from
// a bookmark holding the current port.
func (s PortStatus) NeedsReinstall() bool {
	return s.Port != 0 && s.Bookmarklet != 0 && s.Bookmarklet != s.Port
}

var errNoPort = apperr.New(apperr.CodeConflict, "LeGaJ couldn't open its clipper: no free local port was found. Restart your computer and try again.")

// Listen opens the clip server's listener on 127.0.0.1 only, on the
// persisted port. On first run, or when another program holds the port,
// it picks a new one and persists it.
func Listen(ctx context.Context, meta Meta) (net.Listener, PortStatus, error) {
	var st PortStatus
	var err error
	if st.Bookmarklet, err = InstalledPort(ctx, meta); err != nil {
		return nil, st, err
	}
	v, ok, err := meta.GetMeta(ctx, PortKey)
	if err != nil {
		return nil, st, storageErr(err)
	}
	if port, err := strconv.Atoi(v); ok && err == nil && port >= PortLow && port <= PortHigh {
		if ln, err := listenPort(port); err == nil {
			st.Port = port
			return ln, st, nil
		}
		st.Previous = port
	}
	for i := 0; i < portTries; i++ {
		port := PortLow + rand.IntN(PortHigh-PortLow+1)
		if port == st.Previous {
			continue
		}
		ln, err := listenPort(port)
		if err != nil {
			continue // in use or reserved (Windows reserves port ranges)
		}
		if err := meta.SetMeta(ctx, PortKey, strconv.Itoa(port)); err != nil {
			_ = ln.Close()
			return nil, st, storageErr(err)
		}
		// Any bookmark made so far holds the old port, even with no clip yet
		// to prove it, so the banner must show (R18).
		if st.Previous != 0 && st.Bookmarklet == 0 {
			if err := MarkInstalled(ctx, meta, st.Previous); err != nil {
				_ = ln.Close()
				return nil, st, err
			}
			st.Bookmarklet = st.Previous
		}
		st.Port = port
		return ln, st, nil
	}
	return nil, st, errNoPort
}

// InstalledPort reads the port the installed bookmarklet is taken to have.
// It's 0 when none was installed; a damaged value reads the same way.
func InstalledPort(ctx context.Context, meta Meta) (int, error) {
	v, ok, err := meta.GetMeta(ctx, BookmarkletPortKey)
	if err != nil {
		return 0, storageErr(err)
	}
	if !ok {
		return 0, nil
	}
	port, err := strconv.Atoi(v)
	if err != nil {
		return 0, nil
	}
	return port, nil
}

// MarkInstalled records the port the installed bookmarklet is taken to have.
func MarkInstalled(ctx context.Context, meta Meta, port int) error {
	if err := meta.SetMeta(ctx, BookmarkletPortKey, strconv.Itoa(port)); err != nil {
		return storageErr(err)
	}
	return nil
}

// listenPort binds the loopback address only: nothing on the network can
// reach the clip server (C41).
func listenPort(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

func storageErr(err error) error {
	return apperr.Wrap(err, apperr.CodeStorage, "LeGaJ couldn't read its clipper settings.")
}
