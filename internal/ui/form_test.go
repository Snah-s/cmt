package ui

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Snah-s/cmt/internal/catalog"
	"github.com/Snah-s/cmt/internal/commit"
	"github.com/Snah-s/cmt/internal/config"
)

func testCatalog(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// press sends keys to a model, with the text a terminal would type for printable keys.
func press(model tea.Model, keys ...rune) {
	for _, key := range keys {
		msg := tea.KeyPressMsg{Code: key}
		if unicode.IsPrint(key) {
			msg.Text = string(key)
		}
		model.Update(msg)
	}
}

// fill drives the selectors with keys, the scope and confirmation with lines, and types title and body.
func fill(t *testing.T, m *commitForm, lines, title, body string, keys ...rune) (Draft, string, error) {
	t.Helper()
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	press(selector, keys...)
	if !selector.done {
		t.Fatalf("selectors stopped at %q", selector.step)
	}
	var output bytes.Buffer
	output.WriteString(strings.Join(selector.answers, "\n") + "\n")
	r := &lineReader{reader: strings.NewReader(lines)}
	if err := m.textFields(r, &output); err != nil {
		return Draft{}, output.String(), err
	}
	prompt := m.newTitlePrompt()
	press(prompt, append([]rune(title), tea.KeyEnter)...)
	if !prompt.done {
		t.Fatalf("title prompt rejected %q: %v", prompt.value(), prompt.err)
	}
	m.draft.Description = prompt.value()
	if !m.options.HideBody {
		prompt := newBodyModel(m.options.Editor)
		press(prompt, append([]rune(body), tea.KeyEnter)...)
		if done, err := m.applyBody(prompt, nil, &output); !done || err != nil {
			t.Fatalf("body prompt: done=%v err=%v", done, err)
		}
	}
	err := m.confirmFields(r, &output)
	return m.draft.Active(), output.String(), err
}

func TestPreferencePromptsAndPreview(t *testing.T) {
	c := testCatalog(t)
	original := Draft{Mode: "hybrid", Type: "docs", Emoji: ":memo:", Scope: "api", Description: "update README", Body: "  original\n\nSigned-off-by: Ana\n"}
	opts := Options{FormatExplicit: true, HideScope: true, HideBody: true, MessageOptions: commit.Options{EmojiFormat: "code"}}
	d, output, err := fill(t, newCommitForm(c, original, opts), "y\n", "", "", tea.KeyEnter, tea.KeyEnter)
	if err != nil || d != original || !strings.Contains(output, "docs(api): :memo: update README") || strings.Contains(output, "Scope (") || strings.Contains(output, "Body (optional)") {
		t.Fatalf("hidden prompts/preview: %+v %v / %q", d, err, output)
	}
	opts.HideScope = false
	opts.Scopes = []string{"api", "cli"}
	d, _, err = fill(t, newCommitForm(c, original, opts), "y\n", "", "", tea.KeyEnter, tea.KeyEnter, tea.KeyUp, tea.KeyEnter)
	if err != nil || d.Scope != "" || d.Body != original.Body {
		t.Fatalf("optional configured scope: %+v %v", d, err)
	}
	opts.Scopes = []string{}
	d, output, err = fill(t, newCommitForm(c, original, opts), "y\n", "", "", tea.KeyEnter, tea.KeyEnter, tea.KeyUp, tea.KeyEnter)
	if err != nil || d.Scope != "" || !strings.Contains(output, "Skip scope") || strings.Contains(output, "Scope (optional)") {
		t.Fatalf("empty configured scope list accepted a new scope: %+v %v / %q", d, err, output)
	}
	opts.MessageOptions = commit.Options{EmojiFormat: "code", CapitalizeTitle: false}
	opts.HideScope = true
	d, output, err = fill(t, newCommitForm(c, Draft{Mode: "emoji", Emoji: ":memo:", Description: "update README"}, opts), "y\n", "", "", tea.KeyEnter)
	if err != nil || d.Description != "update README" || !strings.Contains(output, ":memo: update README") {
		t.Fatalf("preview differs from preferences: %+v %v / %q", d, err, output)
	}
}

func TestAccessibleConfiguration(t *testing.T) {
	p, err := Configure(context.Background(), strings.NewReader("hybrid\ncode\nn\n[\"api\",\"cli\"]\nn\nnvim\n\ny\n"), io.Discard, config.Defaults())
	if err != nil || p.CommitFormat != "hybrid" || p.EmojiFormat != "code" || p.CapitalizeTitle || p.MessagePrompt || !p.ScopePrompt || len(p.Scopes) != 2 || p.Editor != "nvim" {
		t.Fatalf("configuration: %+v %v", p, err)
	}
	if _, err := Configure(context.Background(), strings.NewReader(""), io.Discard, config.Defaults()); !errors.Is(err, io.EOF) {
		t.Fatalf("config EOF: %v", err)
	}
}

func TestPreviewAndTypeChange(t *testing.T) {
	c := testCatalog(t)
	d := Draft{Mode: "hybrid", Type: "feat", Emoji: ":sparkles:", Scope: "users", Description: "add résumé search", Body: "first paragraph\n\nSigned-off-by: Ana"}
	if got := commit.Preview(d, c, commit.DefaultOptions()).Text; got != "feat(users): ✨ add résumé search\n\nfirst paragraph\n\nSigned-off-by: Ana" {
		t.Fatalf("preview: %q", got)
	}
	m := newCommitForm(c, d, Options{FormatExplicit: true})
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.draft.Type != "fix" {
		t.Fatalf("keyboard did not change the type: %q", m.draft.Type)
	}
	if selector.step != "emoji" || selector.done || m.draft.Emoji == d.Emoji || m.draft.Body != d.Body || m.draft.Description != d.Description || m.draft.Scope != d.Scope {
		t.Fatalf("type change discarded data, reused an incompatible emoji, or skipped confirmation: %+v", m.draft)
	}
	for _, option := range selector.emojis.options {
		if !c.Compatible("fix", option.entry.Code) {
			t.Fatalf("incompatible option: %q", option.entry.Code)
		}
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !selector.done || !c.Compatible("fix", m.draft.Emoji) {
		t.Fatalf("emoji confirmation did not finish with a compatible choice: %+v", m.draft)
	}
	for _, mode := range []string{"standard", "emoji", "hybrid"} {
		d.Mode = mode
		title := commit.Preview(d, c, commit.DefaultOptions()).Title
		if mode == "standard" && strings.Contains(title, "✨") {
			t.Fatal("standard contains an emoji")
		}
		if mode == "emoji" && title != "✨ (users): Add résumé search" {
			t.Fatalf("emoji: %q", title)
		}
	}
}

func TestEmojiSearchAndEscape(t *testing.T) {
	m := newCommitForm(testCatalog(t), Draft{Mode: "hybrid", Type: "feat"}, Options{FormatExplicit: true})
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	selector.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	selector.Update(tea.KeyPressMsg{Code: 's', Text: "sparkles"})
	if !selector.emojis.filtering || !strings.Contains(ansi.Strip(selector.View().Content), ":sparkles:") {
		t.Fatal("search does not show the emoji")
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if selector.form.State != huh.StateNormal || selector.emojis.filtering {
		t.Fatal("Escape from search canceled the form")
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.draft.Emoji != ":sparkles:" {
		t.Fatalf("search selection: %q", m.draft.Emoji)
	}
}

func TestSmallTerminalAndUnicodeCounter(t *testing.T) {
	m := newCommitForm(testCatalog(t), Draft{Description: strings.Repeat("á", 80)}, Options{})
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	view := ansi.Strip(selector.View().Content)
	if got := utf8.RuneCountInString(m.message().Title); got != 86 {
		t.Fatalf("counter: %d", got)
	}
	if selector.View().AltScreen || !strings.Contains(m.previewStatus(), "86/100") {
		t.Fatalf("alternate screen or incorrect counter: %q", view)
	}
	if got := len(strings.Split(view, "\n")); got > 15 {
		t.Fatalf("view exceeds terminal: %d lines", got)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("line overflows: %q", line)
		}
	}
}

func TestModesAndTextEOF(t *testing.T) {
	c := testCatalog(t)
	for i, mode := range []string{"standard", "emoji", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			keys := append(slices.Repeat([]rune{tea.KeyDown}, i), tea.KeyEnter)
			if mode != "emoji" {
				keys = append(keys, tea.KeyDown, tea.KeyDown, tea.KeyEnter)
			}
			if mode != "standard" {
				keys = append(append(keys, []rune(":memo:")...), tea.KeyEnter)
			}
			m := newCommitForm(c, Draft{}, Options{})
			d, output, err := fill(t, m, "\ny\n", "update guide", "first paragraph", keys...)
			if err != nil || d.Mode != mode || d.Body != "first paragraph" || !strings.Contains(output, m.message().Text) ||
				(mode != "emoji" && d.Type != "docs") || (mode != "standard" && d.Emoji != ":memo:") {
				t.Fatalf("mode answers: %+v %v / %q", d, err, output)
			}
		})
	}
	if err := newCommitForm(c, Draft{}, Options{}).textFields(&lineReader{reader: strings.NewReader("")}, io.Discard); !errors.Is(err, io.EOF) {
		t.Fatalf("text EOF: %v", err)
	}
	if err := newCommitForm(c, Draft{Description: "update guide"}, Options{}).confirmFields(&lineReader{reader: strings.NewReader("n\n")}, io.Discard); !errors.Is(err, ErrCanceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestInlineEmojiInitialSelection(t *testing.T) {
	c := testCatalog(t)
	for _, code := range []string{"", ":memo:"} {
		t.Run(code, func(t *testing.T) {
			owner := newCommitForm(c, Draft{Mode: "emoji", Emoji: code}, Options{})
			selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
			selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			want := code
			if want == "" {
				want = c.Entries[0].Code
			}
			if got, ok := selector.emojis.hovered(); !ok || got != want || selector.step != "emoji" {
				t.Fatalf("initial emoji: got %q, step %q, want %q", got, selector.step, want)
			}
		})
	}
}

func TestInlineSelectorStagesReuseOneCompactList(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{}, Options{})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.form.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if view := ansi.Strip(selector.View().Content); len(strings.Split(view, "\n")) != 4 {
		t.Fatalf("format selector reserved empty rows: %q", view)
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if owner.draft.Mode != "hybrid" || selector.step != "type" {
		t.Fatalf("format selection did not advance: mode=%q step=%q", owner.draft.Mode, selector.step)
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "emoji" {
		t.Fatalf("type selection did not advance to emoji: %q", selector.step)
	}
	view := ansi.Strip(selector.View().Content)
	if strings.Count(view, "Choose an emoji") != 1 || !strings.Contains(view, "> ✨ :sparkles:") {
		t.Fatalf("selector did not start with a real compatible emoji: %q", view)
	}
	if got := strings.Count(view, "> "); got != 1 {
		t.Fatalf("emoji selector rendered %d cursors: %q", got, view)
	}
	if lines := strings.Split(view, "\n"); len(lines) > 9 {
		t.Fatalf("emoji selector used too many rows: %d / %q", len(lines), view)
	}
	for _, background := range []color.Color{color.Black, color.White, color.Black} {
		selector.Update(tea.BackgroundColorMsg{Color: background})
		if got := ansi.Strip(selector.View().Content); got != view {
			t.Fatalf("background change altered the selection: %q", got)
		}
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !selector.done || owner.draft.Emoji != ":sparkles:" || len(selector.answers) != 3 {
		t.Fatalf("emoji selection did not finish: draft=%+v answers=%q", owner.draft, selector.answers)
	}
}

func TestOptionalSingleLineBody(t *testing.T) {
	for _, body := range []string{"", "one paragraph", "  indented body  "} {
		t.Run(body, func(t *testing.T) {
			m := newCommitForm(testCatalog(t), Draft{Mode: "standard", Type: "docs", Body: "old body\n\nSigned-off-by: Ana"}, Options{HideScope: true})
			prompt := newBodyModel("")
			if view := ansi.Strip(prompt.View().Content); !strings.Contains(view, "Body (optional)") || strings.Contains(view, "Ctrl+O") {
				t.Fatalf("body prompt without an editor: %q", view)
			}
			press(prompt, append([]rune(body), tea.KeyEnter)...)
			var output bytes.Buffer
			done, err := m.applyBody(prompt, nil, &output)
			want := body
			if body == "" {
				want = "old body\n\nSigned-off-by: Ana"
			}
			if !done || err != nil || m.draft.Body != want {
				t.Fatalf("body: %q, done=%v, err=%v", m.draft.Body, done, err)
			}
		})
	}
}

func TestConfirmationDescription(t *testing.T) {
	const description = "Creates a Git commit from the index."
	m := newCommitForm(testCatalog(t), Draft{Mode: "standard", Type: "docs", Description: "update guide"}, Options{HideScope: true, HideBody: true, ConfirmDescription: description})
	var output bytes.Buffer
	r := &lineReader{reader: strings.NewReader("y\n")}
	if err := m.textFields(r, &output); err != nil {
		t.Fatal(err)
	}
	if err := m.confirmFields(r, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Count(text, description) != 1 || strings.Index(text, description) > strings.Index(text, "Finish the message?") {
		t.Fatalf("confirmation omitted its effect: %q", text)
	}
}

func TestEmptyHybridCategory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte(`[{"emoji":"🆕","code":":new:","name":"new","description":"New entry."}]`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := catalog.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := newCommitForm(c, Draft{Mode: "hybrid"}, Options{FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "type" || owner.draft.Emoji != "" || !strings.Contains(ansi.Strip(selector.View().Content), "no classified emojis") {
		t.Fatalf("empty category reused type options: step=%q draft=%+v view=%q", selector.step, owner.draft, selector.View().Content)
	}
	// A different category with entries can still be chosen.
	owner.catalog.Types["fix"] = []string{":new:"}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "emoji" || owner.draft.Type != "fix" || owner.draft.Emoji != ":new:" {
		t.Fatalf("compatible category could not be chosen: step=%q draft=%+v", selector.step, owner.draft)
	}
}

func TestElevenTypesInSelector(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{Mode: "hybrid"}, Options{FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	view := ansi.Strip(selector.View().Content)
	if !strings.Contains(view, "feat      A new feature") {
		t.Fatalf("type selector lacks descriptions: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("type line overflows a 40-column terminal: %q", line)
		}
	}
	press(selector, append(slices.Repeat([]rune{tea.KeyDown}, len(commit.Types)-1), tea.KeyEnter)...)
	if owner.draft.Type != "revert" || selector.step != "emoji" {
		t.Fatalf("last type: type=%q step=%q", owner.draft.Type, selector.step)
	}
	var codes []string
	for _, option := range selector.emojis.options {
		codes = append(codes, option.entry.Code)
	}
	if !slices.Equal(codes, []string{":rewind:", ":boom:"}) {
		t.Fatalf("revert emojis: %v", codes)
	}
}

func TestBreakingToggleInTypeSelector(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{Mode: "standard"}, Options{FormatExplicit: true, HideScope: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	if view := ansi.Strip(selector.View().Content); !strings.Contains(view, "!: mark a breaking change") {
		t.Fatalf("missing ! hint: %q", view)
	}
	press(selector, tea.KeyDown, '!')
	if view := ansi.Strip(selector.View().Content); !owner.draft.Breaking || !strings.Contains(view, "> fix!") {
		t.Fatalf("! did not mark the hovered type: breaking=%v view=%q", owner.draft.Breaking, view)
	}
	press(selector, '!', '!', tea.KeyEnter)
	if !selector.done || owner.draft.Type != "fix" || !owner.draft.Breaking || selector.answers[0] != "? Choose a commit type: fix! (breaking change)" {
		t.Fatalf("breaking type: %+v %q", owner.draft, selector.answers)
	}
}

func TestTitleCounterAndLimit(t *testing.T) {
	c := testCatalog(t)
	m := newCommitForm(c, Draft{Mode: "hybrid", Type: "fix", Emoji: ":bug:", Description: "keep"}, Options{FormatExplicit: true})
	prompt := m.newTitlePrompt()
	// "fix: 🐛 " takes 8 of 100 (🐛 counts 2, as in commitlint), leaving 92 for the description.
	if view := strings.TrimRight(ansi.Strip(prompt.View().Content), " "); view != "? Enter the commit title [4/92]: keep" {
		t.Fatalf("counter for the received title: %q", view)
	}
	press(prompt, []rune(strings.Repeat("a", 66))...)
	if view := prompt.View().Content; !strings.Contains(view, sgrYellow+"[66/92]") {
		t.Fatalf("over 72 the counter must turn yellow: %q", view)
	}
	press(prompt, []rune(strings.Repeat("a", 27))...)
	press(prompt, tea.KeyEnter)
	if view := prompt.View().Content; prompt.done || prompt.err == nil || !strings.Contains(view, sgrRed+"[93/92]") {
		t.Fatalf("accepted a 101-character title: %v %q", prompt.err, view)
	}
	press(prompt, tea.KeyBackspace, tea.KeyEnter)
	if !prompt.done || len(prompt.value()) != 92 {
		t.Fatalf("rejected a 100-character title: %v", prompt.err)
	}
	empty := m.newTitlePrompt()
	press(empty, tea.KeyEnter)
	if !empty.done || empty.value() != "keep" {
		t.Fatalf("Enter without typing must keep the received title: %q %v", empty.value(), empty.err)
	}
}
