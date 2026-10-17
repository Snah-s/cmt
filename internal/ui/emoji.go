package ui

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Snah-s/cmt/internal/catalog"
)

type emojiOption struct {
	entry  catalog.Emoji
	label  string
	search string
	words  []string
}

type emojiMatch struct {
	index, score int
}

type emojiSelector struct {
	options   []emojiOption
	matches   []emojiMatch
	cursor    int
	query     string
	filtering bool
	err       error
}

func newEmojiSelector(owner *commitForm) *emojiSelector {
	s := &emojiSelector{
		options: make([]emojiOption, 0, len(owner.catalog.Entries)),
		matches: make([]emojiMatch, 0, len(owner.catalog.Entries)),
	}
	for _, entry := range owner.catalog.Entries {
		if owner.draft.Mode == "hybrid" && !owner.catalog.Compatible(owner.draft.Type, entry.Code) {
			continue
		}
		text := strings.ToLower(entry.Name + " " + entry.Code + " " + entry.Description + " " + entry.Emoji)
		s.options = append(s.options, emojiOption{entry, entry.Emoji + " " + entry.Code + " " + entry.Description, text, strings.Fields(text)})
		s.matches = append(s.matches, emojiMatch{index: len(s.options) - 1})
		if entry.Code == owner.draft.Emoji {
			s.cursor = len(s.options) - 1
		}
	}
	return s
}

func (s *emojiSelector) hovered() (string, bool) {
	if len(s.matches) == 0 {
		return "", false
	}
	return s.options[s.matches[s.cursor].index].entry.Code, true
}

func (m *inlineSelectorModel) updateEmoji(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.emojis
	if paste, ok := msg.(tea.PasteMsg); ok {
		s.typeText(paste.Content)
		return m, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch press.String() {
	case "ctrl+c":
		m.form.State = huh.StateAborted
		return m, tea.Quit
	case "esc":
		if s.filtering {
			previous, _ := s.hovered()
			s.query, s.filtering = "", false
			s.filter()
			for i, match := range s.matches {
				if s.options[match.index].entry.Code == previous {
					s.cursor = i
					break
				}
			}
		} else {
			m.form.State = huh.StateAborted
			return m, tea.Quit
		}
	case "enter":
		value, found := s.hovered()
		if !found {
			return m, nil
		}
		if s.err = m.validate(value); s.err == nil {
			m.owner.draft.Emoji = value
			return m, m.choose()
		}
	case "up", "ctrl+p":
		if len(s.matches) > 0 {
			s.cursor = (s.cursor + len(s.matches) - 1) % len(s.matches)
		}
	case "down", "ctrl+n":
		if len(s.matches) > 0 {
			s.cursor = (s.cursor + 1) % len(s.matches)
		}
	case "backspace", "ctrl+h":
		if s.query != "" {
			_, size := utf8.DecodeLastRuneInString(s.query)
			s.query = s.query[:len(s.query)-size]
			s.filter()
		}
	case "ctrl+u":
		s.query = ""
		s.filter()
	case "/":
		// Keep the previous search shortcut while allowing direct typing.
		if !s.filtering {
			s.filtering = true
		} else {
			s.typeText(press.Text)
		}
	default:
		if press.Mod == 0 || press.Mod == tea.ModShift {
			s.typeText(press.Text)
		}
	}
	return m, nil
}

func (s *emojiSelector) typeText(text string) {
	text = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || r == '\u200d' {
			return r
		}
		return -1
	}, text)
	if text == "" {
		return
	}
	s.filtering = true
	s.query += text
	s.filter()
}

func (s *emojiSelector) filter() {
	s.matches = s.matches[:0]
	s.cursor, s.err = 0, nil
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(s.query)))
	for i, option := range s.options {
		score := 0
		matched := true
		for _, token := range tokens {
			if position := strings.Index(option.search, token); position >= 0 {
				score += position
				continue
			}
			best := -1
			for _, word := range option.words {
				if rank := fuzzyScore(token, word); rank >= 0 && (best < 0 || rank < best) {
					best = rank
				}
			}
			if best < 0 {
				matched = false
				break
			}
			score += 1000 + best
		}
		if matched {
			s.matches = append(s.matches, emojiMatch{i, score})
		}
	}
	slices.SortStableFunc(s.matches, func(a, b emojiMatch) int { return a.score - b.score })
}

// Prefer ordered letters; tolerate one edit in longer words without a search dependency.
func fuzzyScore(query, word string) int {
	remaining := query
	position := 0
	for _, r := range word {
		position++
		want, size := utf8.DecodeRuneInString(remaining)
		if r != want {
			continue
		}
		remaining = remaining[size:]
		if remaining == "" {
			return position - utf8.RuneCountInString(query)
		}
	}
	if utf8.RuneCountInString(query) >= 4 && oneEditApart(query, word) {
		return 100
	}
	return -1
}

func oneEditApart(query, word string) bool {
	a, b := []rune(query), []rune(word)
	if len(a)-len(b) > 1 || len(b)-len(a) > 1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		switch {
		case len(a) > len(b):
			i++
		case len(b) > len(a):
			j++
		case i+1 < len(a) && a[i] == b[j+1] && a[i+1] == b[j]:
			i += 2
			j += 2
		default:
			i++
			j++
		}
	}
	return edits+len(a)-i+len(b)-j <= 1
}

func (s *emojiSelector) view(width, height int) string {
	if width <= 0 {
		width = 80
	}
	rows := min(5, max(1, len(s.options)))
	if height > 0 {
		rows = min(rows, max(1, height-2))
	}
	visible := min(rows, len(s.matches))
	var view strings.Builder
	write := func(line string) {
		if view.Len() > 0 {
			view.WriteByte('\n')
		}
		// Fit both width modes without wrapping the list into extra terminal rows.
		view.WriteString(ansi.TruncateWc(ansi.Truncate(line, max(1, width-1), "…"), max(1, width-1), "…"))
	}
	title := "┃ Choose an emoji"
	if s.filtering {
		title += ": " + s.query
	}
	write(title)
	for row := range visible {
		index := (s.cursor - visible/2 + row + len(s.matches)) % len(s.matches)
		prefix := "┃   "
		if index == s.cursor {
			prefix = "┃ > "
		}
		write(prefix + s.options[s.matches[index].index].label)
	}
	if len(s.matches) == 0 {
		write("┃ No matching emojis")
	}
	for row := max(1, visible); row < rows; row++ {
		view.WriteByte('\n')
	}
	write("  Type to search · ↑/↓ scroll · Enter select · Esc clear/cancel")
	if s.err != nil {
		write(s.err.Error())
	}
	return view.String()
}
