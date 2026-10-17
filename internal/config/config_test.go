package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreferencesAndImport(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `{`, `{"autoAdd":null}`, `{"scopePrompt":null}`, `{"scopePrompt":[null]}`, `{"scopePrompt":["API"]}`, `{"scopePrompt":["two words"]}`, `{"scopePrompt":"api"}`, `{"messagePrompt":0}`, `{"capitalizeTitle":"true"}`, `{"emojiFormat":"unicode"}`, `{"catalogUrl":"file:///tmp/catalog"}`, `{"commitFormat":"unknown"}`, "{\"other\":\"\xff\"}"} {
		if _, err := Parse([]byte(data), false); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	p, err := Parse([]byte(`{"autoAdd":true,"scopePrompt":["api","cli"],"messagePrompt":false,"capitalizeTitle":false,"emojiFormat":"code","extra":{"keep":true},"gitmojisUrl":"https://example.invalid/emojis"}`), true)
	if err != nil || p.CommitFormat != "emoji" || !p.ScopePrompt || len(p.Scopes) != 2 || p.MessagePrompt || p.CapitalizeTitle {
		t.Fatalf("legacy: %+v %v", p, err)
	}
	if p.CatalogURL != "https://example.invalid/emojis" || p.Unknown["gitmojisUrl"] != nil {
		t.Fatalf("legacy catalog URL not migrated: %+v", p)
	}
	root := t.TempDir()
	source := filepath.Join(root, "legacy.json")
	dest := filepath.Join(root, "native", "config.json")
	original, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	// An actual legacy profile has no commitFormat; importing must choose emoji.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(original, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "commitFormat")
	fields["autoAdd"] = json.RawMessage(`true`)
	fields["gitmojisUrl"] = fields["catalogUrl"]
	delete(fields, "catalogUrl")
	original, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := importProfile(source, dest)
	if err != nil {
		t.Fatal(err)
	}
	if imported.CommitFormat != "emoji" || imported.CatalogURL != p.CatalogURL || string(imported.Unknown["extra"]) != "{\"keep\":true}" {
		t.Fatalf("import: %+v", imported)
	}
	before, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("gitmojisUrl")) || bytes.Contains(before, []byte("autoAdd")) || !bytes.Contains(before, []byte("catalogUrl")) {
		t.Fatalf("import saved a retired key: %s", before)
	}
	if _, err := importProfile(source, dest); err == nil {
		t.Fatal("import overwrote existing profile")
	}
	after, _ := os.ReadFile(dest)
	legacy, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) || !bytes.Equal(legacy, original) {
		t.Fatal("import changed a protected file")
	}
	p, err = imported.Set([]string{`commitFormat="hybrid"`, `scopePrompt=false`})
	if err != nil || p.CommitFormat != "hybrid" || p.ScopePrompt || p.Scopes != nil || len(p.Unknown) != 1 {
		t.Fatalf("set: %+v %v", p, err)
	}
	if err := Save(dest, p); err != nil {
		t.Fatal(err)
	}
	for _, assign := range []string{`invalid`, `commitFormat=hybrid`, `autoAdd=null`, `autoAdd=true`, `autoAdd=false`, `unknown=true`} {
		if _, err := p.Set([]string{assign}); err == nil {
			t.Fatalf("accepted %q", assign)
		}
	}
}

func TestRetiredAutoAdd(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		p, err := Parse([]byte(`{"autoAdd":`+value+`}`), false)
		if err != nil || p.Unknown["autoAdd"] != nil {
			t.Fatalf("previous autoAdd profile: %+v %v", p, err)
		}
		data, err := p.JSON()
		if err != nil || bytes.Contains(data, []byte("autoAdd")) {
			t.Fatalf("serialized retired preference: %s %v", data, err)
		}
	}
}

func TestLegacyCatalogURL(t *testing.T) {
	p, err := Parse([]byte(`{"gitmojisUrl":"https://old.invalid/emojis","catalogUrl":"https://new.invalid/emojis"}`), true)
	if err != nil || p.CatalogURL != "https://new.invalid/emojis" {
		t.Fatalf("current catalog key must take precedence: %+v %v", p, err)
	}
	if _, err := Parse([]byte(`{"gitmojisUrl":"file:///tmp/catalog"}`), true); err == nil {
		t.Fatal("import accepted an invalid legacy catalog URL")
	}
}

func TestPathsAndDefaults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("APPDATA", filepath.Join(root, "config"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	path, err := NativePath()
	if err != nil || !filepath.IsAbs(path) {
		t.Fatalf("native path: %q %v", path, err)
	}
	legacy, err := LegacyPath()
	if err != nil || !filepath.IsAbs(legacy) || !strings.Contains(legacy, "gitmoji-nodejs") {
		t.Fatalf("legacy path: %q %v", legacy, err)
	}
	wantNative, wantLegacy := filepath.Join(root, "config", "cmt", "config.json"), filepath.Join(root, "config", "gitmoji-nodejs", "config.json")
	switch runtime.GOOS {
	case "darwin":
		wantNative = filepath.Join(root, "Library", "Application Support", "cmt", "config.json")
		wantLegacy = filepath.Join(root, "Library", "Preferences", "gitmoji-nodejs", "config.json")
	case "windows":
		wantLegacy = filepath.Join(root, "config", "gitmoji-nodejs", "Config", "config.json")
	}
	if path != wantNative || legacy != wantLegacy {
		t.Fatalf("platform paths: native %q, legacy %q", path, legacy)
	}
	original := []byte(`{"emojiFormat":"code","scopePrompt":["cli"],"extra":true}`)
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := importProfile(legacy, path); err != nil || p.CommitFormat != "emoji" || p.EmojiFormat != "code" {
		t.Fatalf("platform import: %+v %v", p, err)
	}
	if data, err := os.ReadFile(legacy); err != nil || !bytes.Equal(data, original) {
		t.Fatal("platform import altered the legacy profile")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r, err := Resolve(string(filepath.Separator))
	if err != nil || r.Source != "defaults" || r.Preferences.CommitFormat != "standard" {
		t.Fatalf("root/defaults: %+v %v", r, err)
	}
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CONFIG_HOME", "relative")
		if _, err := NativePath(); err == nil {
			t.Fatal("relative XDG path accepted")
		}
		if _, err := LegacyPath(); err == nil {
			t.Fatal("relative legacy XDG path accepted")
		}
	}
}

func TestProjectPrecedence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "global"))
	t.Setenv("APPDATA", filepath.Join(root, "global"))
	t.Setenv("HOME", root)
	native, err := NativePath()
	if err != nil {
		t.Fatal(err)
	}
	global, err := Defaults().Set([]string{`messagePrompt=false`, `emojiFormat="code"`})
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(native, global); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	child := filepath.Join(project, "sub", "deep")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Resolve(child)
	if err != nil || r.Project || r.Preferences.MessagePrompt {
		t.Fatalf("global: %+v %v", r, err)
	}
	rc := filepath.Join(project, ".cmtrc.json")
	manifest := filepath.Join(project, "package.json")
	write(rc, `{}`)
	r, err = Resolve(child)
	if err != nil || !r.Project || r.Source != rc || !r.Preferences.MessagePrompt || r.Preferences.EmojiFormat != "emoji" {
		t.Fatalf("empty project: %+v %v", r, err)
	}
	write(manifest, `{"cmt":{"commitFormat":"hybrid"}}`)
	write(rc, `{"invalid`)
	r, err = Resolve(child)
	if err != nil || r.Source != manifest || r.Preferences.CommitFormat != "hybrid" {
		t.Fatalf("manifest priority: %+v %v", r, err)
	}
	write(manifest, `{"name":"example"}`)
	write(rc, `{"commitFormat":"emoji"}`)
	r, err = Resolve(child)
	if err != nil || r.Source != rc {
		t.Fatalf("manifest without key: %+v %v", r, err)
	}
	write(manifest, "\xef\xbb\xbf"+`{"cmt":{"commitFormat":"hybrid"}}`)
	r, err = Resolve(child)
	if err != nil || r.Source != manifest || r.Preferences.CommitFormat != "hybrid" {
		t.Fatalf("manifest with BOM: %+v %v", r, err)
	}
	write(manifest, `{"name": broken`)
	r, err = Resolve(child)
	if err != nil || r.Source != rc {
		t.Fatalf("unrelated broken manifest blocked cmt: %+v %v", r, err)
	}
	nearest := filepath.Join(child, ".cmtrc.json")
	write(nearest, `{"autoAdd":false}`)
	r, err = Resolve(child)
	if err != nil || r.Source != nearest {
		t.Fatalf("nearest: %+v %v", r, err)
	}
	for _, text := range []string{`{"cmt":`, `{"cmt":null}`, `{"cmt":{"scopePrompt":17}}`} {
		path := filepath.Join(child, "package.json")
		write(path, text)
		if _, err := Resolve(child); err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("invalid manifest fell back: %v", err)
		}
	}
	if err := os.Remove(filepath.Join(child, "package.json")); err != nil {
		t.Fatal(err)
	}
	write(nearest, `{"messagePrompt":null}`)
	if _, err := Resolve(child); err == nil || !strings.Contains(err.Error(), nearest) {
		t.Fatalf("invalid rc fell back: %v", err)
	}
	if err := os.Remove(nearest); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(nearest, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(child); err == nil {
		t.Fatal("unreadable candidate fell back")
	}
}

// importProfile follows cmt config --import: validate the legacy file, then create without overwriting.
func importProfile(source, destination string) (Preferences, error) {
	p, err := Read(source, true)
	if err != nil {
		return p, err
	}
	return p, Create(destination, p)
}
