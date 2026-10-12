package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/muesli/cancelreader"

	"github.com/Snah-s/cmt/internal/commit"
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

func (m *commitForm) accessibleFields(input io.Reader, output io.Writer) error {
	r := &lineReader{reader: input}
	if err := m.selectionFields(r, output); err != nil {
		return err
	}
	return m.textFields(r, output)
}

// Huh's numeric select indexes its options before checking read errors.
func accessibleSelect(r *lineReader, output io.Writer, title string, value *string, options []huh.Option[string], validate func(string) error) error {
	if len(options) == 0 {
		return errors.New("no options available for " + title)
	}
	if _, err := fmt.Fprintln(output, title); err != nil {
		return err
	}
	choice := "1"
	for i, option := range options {
		if _, err := fmt.Fprintf(output, "%d. %s\n", i+1, option.Key); err != nil {
			return err
		}
		if option.Value == *value {
			choice = strconv.Itoa(i + 1)
		}
	}
	field := huh.NewInput().Title(fmt.Sprintf("Enter a number between 1 and %d:", len(options))).Value(&choice).Validate(func(input string) error {
		if input == "" {
			input = choice
		}
		index, err := strconv.Atoi(input)
		if err != nil || index < 1 || index > len(options) {
			return errors.New("choose one of the listed numbers")
		}
		if validate != nil {
			return validate(options[index-1].Value)
		}
		return nil
	})
	if err := accessibleField(r, output, field); err != nil {
		return err
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(options) {
		return errors.New("choose one of the listed numbers")
	}
	*value = options[index-1].Value
	return nil
}

func (m *commitForm) selectionFields(r *lineReader, output io.Writer) error {
	if !m.fixedMode {
		if err := accessibleSelect(r, output, "Choose a format", &m.draft.Mode, huh.NewOptions("standard", "emoji", "hybrid"), nil); err != nil {
			return err
		}
	}
	if m.draft.Mode != "emoji" {
		if err := accessibleSelect(r, output, "Choose a commit type", &m.draft.Type, huh.NewOptions(commit.Types...), m.validateType); err != nil {
			return err
		}
	}
	reconcile(&m.draft, m.catalog)
	if m.draft.Mode != "standard" {
		if err := accessibleSelect(r, output, "Choose an emoji", &m.draft.Emoji, m.emojiOptions(), nil); err != nil {
			return err
		}
	}
	if !m.options.HideScope && m.options.Scopes != nil {
		scopeOptions := []huh.Option[string]{huh.NewOption("Skip scope", "")}
		for _, choice := range m.scopeChoices() {
			scopeOptions = append(scopeOptions, huh.NewOption(choice, choice))
		}
		if err := accessibleSelect(r, output, "Choose a scope (optional)", &m.draft.Scope, scopeOptions, commit.ValidateScope); err != nil {
			return err
		}
	}
	return nil
}
