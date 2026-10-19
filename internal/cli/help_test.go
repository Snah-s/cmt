package cli

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var updateMan = flag.Bool("update", false, "rewrite man/cmt.1")

func TestHelp(t *testing.T) {
	overview := Help("")
	for _, c := range commands {
		if !strings.Contains(overview, "\n  "+c.name) {
			t.Errorf("overview misses %s", c.name)
		}
		if text := Help(c.name); !strings.HasPrefix(text, "Usage: "+c.usage+"\n") {
			t.Errorf("%s help: %q", c.name, text)
		}
	}
	for args, want := range map[string]string{"prototype --help": "Usage: cmt prototype", "-c -h": "Usage: cmt commit", "--hook=x -h": "Usage: cmt hook", "-h": "Usage: cmt <command>"} {
		o, err := Parse(strings.Fields(args))
		if err != nil || !strings.HasPrefix(HelpFor(o), want) {
			t.Errorf("%s: %v %q", args, err, HelpFor(o))
		}
	}
	// Every flag the parser accepts is documented.
	for _, name := range []string{"--format", "--type", "--breaking", "--emoji", "--title", "--scope", "--message", "--accessible", "--hook", "--show", "--set", "--import", "--help", "--version"} {
		if !strings.Contains(Man(), strings.ReplaceAll(name, "-", `\-`)) {
			t.Errorf("man page misses %s", name)
		}
	}
}

// The packaged man page is generated: go test ./internal/cli -run TestManPage -update
func TestManPage(t *testing.T) {
	const path = "../../man/cmt.1"
	if *updateMan {
		if err := os.WriteFile(path, []byte(Man()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != Man() {
		t.Fatalf("%s is out of date (%v); regenerate it with -update", path, err)
	}
}
