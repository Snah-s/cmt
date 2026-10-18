package main

import (
	"encoding/json"
	"os"

	"github.com/Snah-s/cmt/internal/catalog"
	"github.com/Snah-s/cmt/internal/commit"
	"github.com/Snah-s/cmt/internal/storage"
)

// savedDraft is the last confirmed message of a worktree, kept until it reaches a commit.
type savedDraft struct {
	Head  string       `json:"head"`
	Draft commit.Draft `json:"draft"`
}

func saveDraft(path, head string, d commit.Draft) error {
	data, err := json.Marshal(savedDraft{head, d})
	if err != nil {
		return err
	}
	return storage.WriteFile(path, data, 0600)
}

// loadDraft returns a pending draft. One saved at another HEAD (its commit succeeded, or a
// different one did), unreadable or no longer valid for the catalog is removed instead.
func loadDraft(path, head string, c catalog.Catalog, options commit.Options) (commit.Draft, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return commit.Draft{}, false
	}
	var saved savedDraft
	if json.Unmarshal(data, &saved) != nil || saved.Head != head {
		_ = os.Remove(path)
		return commit.Draft{}, false
	}
	if _, err := commit.Build(saved.Draft, c, options); err != nil {
		_ = os.Remove(path)
		return commit.Draft{}, false
	}
	return saved.Draft, true
}
