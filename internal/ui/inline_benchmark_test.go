package ui

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Snah-s/cmt/internal/catalog"
)

func BenchmarkInlineSelectorStart(b *testing.B) {
	c, err := catalog.Load()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		owner := newCommitForm(c, Draft{Mode: "emoji"}, Options{InlineSelectors: true, FormatExplicit: true})
		selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
		selector.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
		selector.View()
	}
}

func BenchmarkInlineSelectorNavigation(b *testing.B) {
	c, err := catalog.Load()
	if err != nil {
		b.Fatal(err)
	}
	owner := newCommitForm(c, Draft{Mode: "emoji"}, Options{InlineSelectors: true, FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		selector.View()
	}
}
