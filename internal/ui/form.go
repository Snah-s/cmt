package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"charm.land/huh/v2"

	"github.com/Snah-s/cmt/internal/catalog"
	"github.com/Snah-s/cmt/internal/commit"
)

var ErrCanceled = huh.ErrUserAborted

type Draft = commit.Draft

type commitForm struct {
	draft     Draft
	catalog   catalog.Catalog
	confirmed bool
	fixedMode bool
	options   Options
}

type Options struct {
	FormatExplicit      bool
	HideScope, HideBody bool
	Scopes              []string
	MessageOptions      commit.Options
	ConfirmDescription  string
	Editor              string
}

func newCommitForm(c catalog.Catalog, draft Draft, options Options) *commitForm {
	if draft.Mode == "" {
		draft.Mode = "standard"
	}
	if draft.Type == "" {
		draft.Type = "feat"
	}
	if options.MessageOptions.EmojiFormat == "" {
		options.MessageOptions = commit.DefaultOptions()
	}
	if options.ConfirmDescription == "" {
		options.ConfirmDescription = "Returns the message only; does not create a commit."
	}
	reconcile(&draft, c)
	return &commitForm{draft: draft, catalog: c, fixedMode: options.FormatExplicit, options: options}
}

func (m *commitForm) validateType(kind string) error {
	if !slices.Contains(commit.Types, kind) {
		return errors.New("choose one of the listed types")
	}
	if m.draft.Mode == "hybrid" && len(m.catalog.Types[kind]) == 0 {
		return fmt.Errorf("%s has no classified emojis in this catalog; choose another type or use standard/emoji mode", kind)
	}
	return nil
}

func (m *commitForm) message() commit.Message {
	return commit.Preview(m.draft, m.catalog, m.options.MessageOptions)
}

func (m *commitForm) scopeChoices() []string {
	choices := slices.Clone(m.options.Scopes)
	if m.draft.Scope != "" && !slices.Contains(choices, m.draft.Scope) {
		choices = append(choices, m.draft.Scope)
	}
	return choices
}

func (m *commitForm) previewStatus() string {
	count := m.message().TitleLength
	if count > 72 {
		return fmt.Sprintf("Preview · %d/72 · warning: long title", count)
	}
	return fmt.Sprintf("Preview · %d/72 Unicode code points", count)
}

func Run(ctx context.Context, input io.Reader, output io.Writer, c catalog.Catalog, draft Draft, options Options) (Draft, error) {
	m := newCommitForm(c, draft, options)
	if err := m.inlineFields(ctx, input, output); err != nil {
		return Draft{}, err
	}
	return m.draft.Active(), nil
}

func reconcile(d *Draft, c catalog.Catalog) {
	if d.Mode == "standard" || (d.Mode == "hybrid" && !c.Compatible(d.Type, d.Emoji)) {
		d.Emoji = ""
	}
}

func (m *commitForm) textFields(r *lineReader, output io.Writer) error {
	run := func(field huh.Field) error { return accessibleField(r, output, field) }
	if !m.options.HideScope && m.options.Scopes == nil {
		if err := run(huh.NewInput().Title("Scope (optional)").Value(&m.draft.Scope).Validate(commit.ValidateScope)); err != nil {
			return err
		}
	}
	return run(huh.NewInput().Title("Enter the commit title").Value(&m.draft.Description).Validate(func(v string) error {
		if v == "" {
			v = m.draft.Description
		}
		return commit.ValidateDescription(v)
	}))
}

func (m *commitForm) confirmFields(r *lineReader, output io.Writer) error {
	run := func(field huh.Field) error { return accessibleField(r, output, field) }
	if err := commit.ValidateBody(m.draft.Body); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "%s\n%s\n%s\n", m.previewStatus(), m.message().Text, m.options.ConfirmDescription); err != nil {
		return err
	}
	if err := run(huh.NewConfirm().Title("Finish the message?").Value(&m.confirmed)); err != nil {
		return err
	}
	if !m.confirmed {
		return ErrCanceled
	}
	return nil
}
