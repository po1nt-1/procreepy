package cli

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
)

// ANSI SGR sequences for the painted levels. INFO is deliberately left
// unpainted: it is the noise floor of a healthy run, and coloring it would
// make the output feel alarming.
const (
	ansiReset = "\x1b[0m"
	ansiWarn  = "\x1b[33m"   //  yellow: noticeable
	ansiError = "\x1b[1;31m" // bold red: maximal attention
)

// colorCodes maps the paintable levels to their SGR sequence. Levels absent
// from the map (INFO, DEBUG, custom) pass through untouched.
var colorCodes = map[slog.Level]string{
	slog.LevelWarn:  ansiWarn,
	slog.LevelError: ansiError,
}

// colorHandler paints the level token of each record and delegates everything
// else to the plain TextHandler, so colored output differs from plain output
// only in the SGR sequences around the level — nothing shifts, nothing else
// is touched. Safe for concurrent use.
type colorHandler struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	text slog.Handler
	opts *slog.HandlerOptions
	// ops is the WithAttrs/WithGroup chain applied to this handler. Children
	// replay it (plus their new op) on their own TextHandler over their own
	// buffer, so the accumulated state of the whole chain is preserved.
	ops []handlerOp
	w   io.Writer
}

// handlerOp is one link of the WithAttrs/WithGroup chain: a pushed group or
// a batch of attributes added at that point in the chain.
type handlerOp struct {
	group string
	attrs []slog.Attr
}

func newColorHandler(opts *slog.HandlerOptions, w io.Writer) *colorHandler {
	c := &colorHandler{w: w, opts: opts}
	c.buf.Reset()
	c.text = slog.NewTextHandler(&c.buf, opts)
	return c
}

// newTextOver rebuilds a plain TextHandler with the given chain of ops on top
// of buf, reproducing exactly the attribute/group state a stdlib handler
// would have reached through the same WithAttrs/WithGroup sequence.
func newTextOver(buf *bytes.Buffer, opts *slog.HandlerOptions, ops []handlerOp) slog.Handler {
	var h slog.Handler = slog.NewTextHandler(buf, opts)
	for _, op := range ops {
		if op.group != "" {
			h = h.WithGroup(op.group)
		} else {
			h = h.WithAttrs(op.attrs)
		}
	}
	return h
}

func (c *colorHandler) child(op handlerOp) *colorHandler {
	ops := make([]handlerOp, len(c.ops)+1)
	copy(ops, c.ops)
	ops[len(c.ops)] = op
	child := &colorHandler{w: c.w, opts: c.opts, ops: ops}
	child.buf.Reset()
	child.text = newTextOver(&child.buf, c.opts, ops)
	return child
}

func (c *colorHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return c.text.Enabled(ctx, l)
}

// WithAttrs and WithGroup build the child around its OWN TextHandler writing
// into its OWN buffer, replaying the parent's whole chain first. Delegating
// to the parent's handler instead would send the record into the parent's
// buffer, which this handler's Handle never reads, and silently drop every
// record logged through the child.
func (c *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return c
	}
	return c.child(handlerOp{attrs: attrs})
}

func (c *colorHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return c
	}
	return c.child(handlerOp{group: name})
}

func (c *colorHandler) Handle(ctx context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Reset()
	if err := c.text.Handle(ctx, r); err != nil {
		return err
	}
	b := c.buf.Bytes()
	code, paint := colorCodes[r.Level]
	if !paint {
		_, err := c.w.Write(b)
		return err
	}
	// The TextHandler always starts a record with the level attribute; find
	// where its value ends (a space, or the closing quote if quoted) and wrap
	// only that value in SGR codes.
	rest, ok := bytes.CutPrefix(b, []byte("level="))
	if !ok {
		// Unrecognized record shape (a future slog, perhaps): emit as-is.
		_, err := c.w.Write(b)
		return err
	}
	i := len(rest)
	if rest[0] == '"' {
		if j := bytes.IndexByte(rest[1:], '"'); j < 0 {
			_, err := c.w.Write(b)
			return err
		} else {
			i = j + 2
		}
	} else if j := bytes.IndexByte(rest, ' '); j >= 0 {
		i = j
	}
	if _, err := c.w.Write([]byte("level=")); err != nil {
		return err
	}
	if _, err := c.w.Write([]byte(code)); err != nil {
		return err
	}
	if _, err := c.w.Write(rest[:i]); err != nil {
		return err
	}
	if _, err := c.w.Write([]byte(ansiReset)); err != nil {
		return err
	}
	_, err := c.w.Write(rest[i:])
	return err
}
