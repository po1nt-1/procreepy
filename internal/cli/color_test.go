package cli

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// plainOpts mirrors the handler options newLogger installs, minus the level
// override, so expectations are rendered with the very same TextHandler the
// app uses.
func plainOpts() *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, v slog.Attr) slog.Attr {
			if v.Key == "time" {
				return slog.Attr{}
			}
			return v
		},
	}
}

func renderPlain(t *testing.T, level slog.Level, msg string, kv ...any) string {
	t.Helper()
	var b bytes.Buffer
	h := slog.NewTextHandler(&b, plainOpts())
	rec := slog.NewRecord(time.Time{}, level, msg, 0)
	for i := 0; i+1 < len(kv); i += 2 {
		rec.AddAttrs(slog.Any(kv[i].(string), kv[i+1]))
	}
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The paintable levels get exactly the level token wrapped; everything else
// is the plain TextHandler output, byte for byte.
func TestColorHandlerPaintsLevels(t *testing.T) {
	tt := []struct {
		name  string
		level slog.Level
		msg   string
		want  string
	}{
		{"info_plain", slog.LevelInfo, "hello",
			"level=INFO msg=hello\n"},
		{"warn_yellow", slog.LevelWarn, "careful",
			"level=\x1b[33mWARN\x1b[0m msg=careful\n"},
		{"error_bold_red", slog.LevelError, "boom",
			"level=\x1b[1;31mERROR\x1b[0m msg=boom\n"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var w bytes.Buffer
			l := slog.New(newColorHandler(plainOpts(), &w))
			switch tc.level {
			case slog.LevelInfo:
				l.Info(tc.msg)
			case slog.LevelWarn:
				l.Warn(tc.msg)
			case slog.LevelError:
				l.Error(tc.msg)
			}
			if w.String() != tc.want {
				t.Errorf("stderr = %q, want %q", w.String(), tc.want)
			}
		})
	}
}

// Attributes keep their TextHandler quoting; only the level is touched.
func TestColorHandlerKeepsQuoting(t *testing.T) {
	var w bytes.Buffer
	slog.New(newColorHandler(plainOpts(), &w)).
		Warn("file conversion failed", "input", `a b (c).procreate`, "err", "seg fault")
	want := "level=\x1b[33mWARN\x1b[0m msg=\"file conversion failed\" " +
		"input=\"a b (c).procreate\" err=\"seg fault\"\n"
	if w.String() != want {
		t.Errorf("stderr = %q, want %q", w.String(), want)
	}
}

// The handler's level threshold (the -q behavior) still applies.
func TestColorHandlerRespectsThreshold(t *testing.T) {
	opts := plainOpts()
	opts.Level = slog.LevelWarn
	var w bytes.Buffer
	l := slog.New(newColorHandler(opts, &w))
	l.Info("hidden")
	l.Warn("shown")
	if want := "level=\x1b[33mWARN\x1b[0m msg=shown\n"; w.String() != want {
		t.Errorf("stderr = %q, want %q", w.String(), want)
	}
}

// newLogger gates the color handler: buffers and non-terminal files stay
// plain, NO_COLOR wins, and an approved terminal gets the color handler.
func TestNewLoggerColorGate(t *testing.T) {
	real := colorOK
	t.Cleanup(func() { colorOK = real })

	t.Run("buffer_stays_plain", func(t *testing.T) {
		colorOK = func(*os.File) bool { panic("must not be consulted") }
		if _, ok := newLogger(&bytes.Buffer{}, false).Handler().(*slog.TextHandler); !ok {
			t.Error("expected the plain TextHandler for a non-file stderr")
		}
	})

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	t.Run("pipe_without_color_approval_stays_plain", func(t *testing.T) {
		colorOK = func(*os.File) bool { return false }
		if _, ok := newLogger(pw, false).Handler().(*slog.TextHandler); !ok {
			t.Error("expected the plain TextHandler for a non-terminal stderr")
		}
	})

	t.Run("approved_terminal_gets_colors", func(t *testing.T) {
		colorOK = func(*os.File) bool { return true }
		l := newLogger(pw, false)
		if _, ok := l.Handler().(*colorHandler); !ok {
			t.Fatal("expected the color handler for an approved terminal stderr")
		}
		// And the painted bytes flow out, end to end.
		l.Error("boom")
		var line []byte
		buf := make([]byte, 1)
		for len(line) == 0 || line[len(line)-1] != '\n' {
			n, err := pr.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			line = append(line, buf[:n]...)
		}
		if string(line) != "level=\x1b[1;31mERROR\x1b[0m msg=boom\n" {
			t.Errorf("stderr = %q", line)
		}
	})
}

// NO_COLOR, once set (even to an empty-but-present value it must not be
// relied upon — any non-empty value vetoes), disables the approval.
func TestColorOKNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Close()
	if colorOK(pw) {
		t.Error("NO_COLOR must disable color regardless of the terminal")
	}
}

// Records logged through logger.With must reach the writer at all: the child
// handler keeps the plain TextHandler shape (attributes quoted, order intact)
// and the paintable levels still get their SGR codes. Before the child got
// its own buffer every one of these records was silently dropped.
func TestColorHandlerWithAttrs(t *testing.T) {
	t.Run("info_carries_attrs_plain", func(t *testing.T) {
		var w bytes.Buffer
		slog.New(newColorHandler(plainOpts(), &w)).With("file", "a b").Info("hello")
		want := renderPlain(t, slog.LevelInfo, "hello", "file", "a b")
		if w.String() != want {
			t.Errorf("stderr = %q, want %q", w.String(), want)
		}
	})

	t.Run("warn_carries_attrs_painted", func(t *testing.T) {
		var w bytes.Buffer
		slog.New(newColorHandler(plainOpts(), &w)).With("tag", "x").Warn("careful")
		want := "level=\x1b[33mWARN\x1b[0m msg=careful tag=x\n"
		if w.String() != want {
			t.Errorf("stderr = %q, want %q", w.String(), want)
		}
	})

	t.Run("nested_with_accumulates", func(t *testing.T) {
		var w bytes.Buffer
		slog.New(newColorHandler(plainOpts(), &w)).With("a", 1).With("b", 2).Warn("m")
		want := "level=\x1b[33mWARN\x1b[0m msg=m a=1 b=2\n"
		if w.String() != want {
			t.Errorf("stderr = %q, want %q", w.String(), want)
		}
	})

	t.Run("parent_and_child_interleaved_no_loss", func(t *testing.T) {
		var w bytes.Buffer
		base := slog.New(newColorHandler(plainOpts(), &w))
		child := base.With("tag", "x")
		same := base.With() // no attrs: identity, must still log
		base.Info("parent")
		child.Info("child")
		same.Info("same")
		base.Warn("again")
		want := "level=INFO msg=parent\n" +
			"level=INFO msg=child tag=x\n" +
			"level=INFO msg=same\n" +
			"level=\x1b[33mWARN\x1b[0m msg=again\n"
		if w.String() != want {
			t.Errorf("stderr = %q, want %q", w.String(), want)
		}
	})
}

// Same guarantee for WithGroup: the record reaches the writer, group keys
// keep the TextHandler `group.key` shape, and stripping the SGR codes yields
// exactly the plain TextHandler output for the same record.
func TestColorHandlerWithGroup(t *testing.T) {
	var w bytes.Buffer
	slog.New(newColorHandler(plainOpts(), &w)).WithGroup("net").Warn("dial", "host", "a b", "err", "refused")
	want := "level=\x1b[33mWARN\x1b[0m msg=dial net.host=\"a b\" net.err=refused\n"
	if w.String() != want {
		t.Fatalf("stderr = %q, want %q", w.String(), want)
	}

	var pb bytes.Buffer
	slog.New(slog.NewTextHandler(&pb, plainOpts())).WithGroup("net").Warn("dial", "host", "a b", "err", "refused")
	repl := strings.NewReplacer(ansiWarn, "", ansiError, "", ansiReset, "")
	stripped := repl.Replace(w.String())
	if stripped != pb.String() {
		t.Errorf("colored-minus-SGR = %q, plain TextHandler = %q", stripped, pb.String())
	}

	var w2 bytes.Buffer
	slog.New(newColorHandler(plainOpts(), &w2)).WithGroup("db").Error("failed")
	if want := "level=\x1b[1;31mERROR\x1b[0m msg=failed\n"; w2.String() != want {
		t.Errorf("stderr = %q, want %q", w2.String(), want)
	}

	t.Run("attrs_before_group_keep_their_prefix", func(t *testing.T) {
		var w bytes.Buffer
		slog.New(newColorHandler(plainOpts(), &w)).With("top", 1).WithGroup("g").Warn("m", "b", 2)
		want := "level=\x1b[33mWARN\x1b[0m msg=m top=1 g.b=2\n"
		if w.String() != want {
			t.Errorf("stderr = %q, want %q", w.String(), want)
		}
	})
}

// Non-TTY path: newLogger installs the plain TextHandler, and With-derived
// loggers on it must stay byte-for-byte that handler's output (no SGR).
func TestWithOnPlainLoggerStaysByteExact(t *testing.T) {
	var w bytes.Buffer
	newLogger(&w, false).With("tag", "x").Warn("careful", "k", "v")
	if bytes.Contains(w.Bytes(), []byte("\x1b[")) {
		t.Errorf("unexpected SGR sequence on the non-TTY path: %q", w.String())
	}
	var pb bytes.Buffer
	slog.New(slog.NewTextHandler(&pb, plainOpts())).With("tag", "x").Warn("careful", "k", "v")
	if w.String() != pb.String() {
		t.Errorf("non-TTY = %q, plain TextHandler = %q", w.String(), pb.String())
	}
}
