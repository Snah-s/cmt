package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Snah-s/cmt/internal/catalog"
	"github.com/Snah-s/cmt/internal/commit"
)

func TestDraftValidity(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	options := commit.DefaultOptions()
	path := filepath.Join(t.TempDir(), "cmt-draft.json")
	d := commit.Draft{Mode: "hybrid", Type: "feat", Emoji: ":boom:", Scope: "api", Description: "drop keys", Body: "detail\n\nSigned-off-by: Ana", Breaking: true}
	if err := saveDraft(path, "abc", d); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("draft must be private: %v %v", info, err)
	}
	if got, ok := loadDraft(path, "abc", c, options); !ok || got != d {
		t.Fatalf("pending draft: %+v %v", got, ok)
	}
	for name, setup := range map[string]func() string{
		"HEAD moved":    func() string { _ = saveDraft(path, "abc", d); return "def" },
		"corrupt":       func() string { _ = os.WriteFile(path, []byte("{"), 0600); return "abc" },
		"invalid draft": func() string { _ = saveDraft(path, "abc", commit.Draft{Mode: "standard", Type: "wip"}); return "abc" },
	} {
		head := setup()
		if _, ok := loadDraft(path, head, c, options); ok {
			t.Fatalf("%s: draft offered", name)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: stale draft kept: %v", name, err)
		}
	}
	if _, ok := loadDraft(path, "abc", c, options); ok {
		t.Fatal("missing draft offered")
	}
}
