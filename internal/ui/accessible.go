package ui

import (
	"context"
	"io"
	"strings"

	"charm.land/huh/v2"
	"github.com/muesli/cancelreader"
)

// Huh creates a scanner per field: limiting reads avoids consuming the next
// answer and detects EOF that its accessible form does not propagate.
type lineReader struct {
	reader   io.Reader
	err      error
	line     strings.Builder
	lastLine string
}

func (r *lineReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		if p[0] == '\n' {
			r.lastLine = strings.TrimSuffix(r.line.String(), "\r")
			r.line.Reset()
		} else {
			r.line.WriteByte(p[0])
		}
	}
	if err != nil {
		r.err = err
	}
	return n, err
}

func runAccessible(ctx context.Context, input io.Reader, run func(io.Reader) error) error {
	reader, err := cancelreader.NewReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- run(reader) }()
	select {
	case <-ctx.Done():
		reader.Cancel()
		<-result
		return ErrCanceled
	case err := <-result:
		return err
	}
}

func accessibleField(reader *lineReader, output io.Writer, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(true).WithInput(reader).WithOutput(output)
	if err := form.Run(); err != nil {
		return err
	}
	return reader.err
}
