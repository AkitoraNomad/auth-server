package internal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func init() {
	level := strings.ToUpper(os.Getenv("LOG_LEVEL"))
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	handler := newColorHandler(os.Stderr, &slog.HandlerOptions{Level: l})
	slog.SetDefault(slog.New(handler))

	if level != "" {
		slog.Info("log level configured", "level", level)
	}
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

var levelColors = map[slog.Level]string{
	slog.LevelDebug: colorCyan,
	slog.LevelInfo:  colorBlue,
	slog.LevelWarn:  colorYellow,
	slog.LevelError: colorRed,
}

var levelNames = map[slog.Level]string{
	slog.LevelDebug: "DEBUG",
	slog.LevelInfo:  "INFO ",
	slog.LevelWarn:  "WARN ",
	slog.LevelError: "ERROR",
}

type colorHandler struct {
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
	group string
}

func newColorHandler(w io.Writer, opts *slog.HandlerOptions) *colorHandler {
	h := &colorHandler{w: w}
	if opts != nil {
		h.level = opts.Level
	}
	if h.level == nil {
		h.level = slog.LevelInfo
	}
	return h
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	var buf strings.Builder

	// timestamp
	buf.WriteString(colorGray)
	buf.WriteString(r.Time.Format("15:04:05.000"))
	buf.WriteString(" ")

	// level
	c := levelColors[r.Level]
	if c == "" {
		c = colorReset
	}
	buf.WriteString(c)
	buf.WriteString(levelNames[r.Level])
	buf.WriteString(colorReset)
	buf.WriteString(" ")

	// message
	buf.WriteString(r.Message)

	// pre-attrs
	for _, a := range h.attrs {
		buf.WriteString(" ")
		buf.WriteString(colorGray)
		buf.WriteString(a.Key)
		buf.WriteString("=")
		buf.WriteString(colorReset)
		buf.WriteString(fmt.Sprint(a.Value.Any()))
	}

	// record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf.WriteString(" ")
		buf.WriteString(colorGray)
		buf.WriteString(a.Key)
		buf.WriteString("=")
		buf.WriteString(colorReset)
		buf.WriteString(fmt.Sprint(a.Value.Any()))
		return true
	})

	buf.WriteString("\n")
	_, err := io.WriteString(h.w, buf.String())
	return err
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = append([]slog.Attr{}, h.attrs...)
	h2.attrs = append(h2.attrs, attrs...)
	return &h2
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	h2 := *h
	h2.group = name
	return &h2
}
