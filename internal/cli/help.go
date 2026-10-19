package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

type entry struct{ name, text string }

type command struct {
	name, aliases, usage, summary string
	flags                         []entry
	notes                         []string
	message, preferences          bool // message: takes messageFlags and shows the prompt keys
}

var messageFlags = []entry{
	{"--format emoji|standard|hybrid", "Message mode; overrides the commitFormat preference"},
	{"--type TYPE", "feat, fix, docs, refactor, test, chore, perf, style, build, ci or revert"},
	{"--breaking", "Mark a breaking change: type! (or press ! in the type selector)"},
	{"--emoji :code:", "Exact catalog code (emoji and hybrid modes)"},
	{"--title DESCRIPTION", "Description, without the type, scope or emoji prefix"},
	{"--scope SCOPE", "Optional scope, lowercase"},
	{"--message BODY", "Body: literal paragraphs and trailers"},
	{"--accessible", "Compatibility alias; prompts are inline by default"},
}

var commands = []command{
	{name: "commit", aliases: "-c", usage: "cmt commit [flags]", summary: "Create a Git commit from the staged changes",
		message: true, notes: []string{
			"Only changes already staged with git add are committed; cmt never stages files.",
			"In a terminal, prompts ask for what the flags leave out, and you confirm before Git runs.",
			"Without a terminal, --title and a complete selection authorize the commit.",
			"If Git fails after you confirm (a hook, the signature...), the message is kept: the next commit asks \"Reuse it? [Y/n]\".",
			"An active managed hook blocks this command; use git commit instead.",
		}},
	{name: "prototype", usage: "cmt prototype [flags]", summary: "Print a message without changing Git or files",
		flags:   []entry{{"--hook FILE", "Review FILE as the hook would, and print the result instead of saving it"}},
		message: true, notes: []string{"Without a terminal, prints the message built from the flags."}},
	{name: "config", aliases: "-g", usage: "cmt config [--show | --set KEY=JSON... | --import FILE]", summary: "Edit global preferences",
		flags: []entry{
			{"--show", "Show global and effective preferences and their source"},
			{"--set KEY=JSON", "Save a global preference; repeat to save several"},
			{"--import FILE", "Import a gitmoji-cli profile when no native profile exists"},
		}, preferences: true, notes: []string{
			"Without flags, opens the preference editor in a terminal, or shows the preferences without one.",
			"The nearest project file overrides global preferences: the \"cmt\" key of package.json, then .cmtrc.json, in each directory up to the root.",
			"Example: cmt config --set 'commitFormat=\"hybrid\"' --set 'customScopes=[\"api\"]'",
		}},
	{name: "list", aliases: "-l", usage: "cmt list", summary: "List the offline emoji catalog"},
	{name: "search", aliases: "-s", usage: "cmt search QUERY...", summary: "Search the catalog for each query independently",
		notes: []string{"Put -- before queries that start with a dash."}},
	{name: "update", aliases: "-u", usage: "cmt update", summary: "Download the catalog from catalogUrl; keep the cache on failure",
		notes: []string{"The only command that uses the network."}},
	{name: "init", aliases: "-i", usage: "cmt init", summary: "Install or update the managed hook in this repository",
		notes: []string{
			"The hook records the absolute path of this executable; run init again after moving it.",
			"Not available on Windows yet.",
		}},
	{name: "remove", aliases: "-r", usage: "cmt remove", summary: "Remove the managed hook; other hooks are never touched"},
	{name: "hook", aliases: "--hook FILE", usage: "cmt hook FILE [SOURCE [OBJECT]]", summary: "Review a commit message file; run by the managed hook",
		message: true, notes: []string{
			"Git passes FILE and SOURCE; you normally run git commit, not this command.",
			"The file is kept unchanged on cancel, without a terminal, during a rebase, and for amend, merge or -c/-C sources.",
		}},
}

var keys = []entry{
	{"Up/Down, Enter", "Move and choose in a selector"},
	{"typing", "Filter the selector (emojis also match codes and descriptions)"},
	{"!", "In the type selector: mark a breaking change"},
	{"Esc", "Clear the filter, or cancel"},
	{"Ctrl+O", "In the body prompt: write the body in an editor"},
	{"Ctrl+C", "Cancel"},
}

var preferences = []entry{
	{"commitFormat", `"standard" (default), "emoji" or "hybrid"`},
	{"scopePrompt", `true (default) suggests scopes; false hides the question; ["api","cli"] allows only that list`},
	{"customScopes", `Extra scope suggestions, such as ["auth","deps"]`},
	{"messagePrompt", "true (default) asks for a body"},
	{"emojiFormat", `"emoji" (default) writes Unicode; "code" writes :code:`},
	{"capitalizeTitle", "true (default) capitalizes emoji-mode titles"},
	{"editor", `Program for Ctrl+O; default: the first of nano, micro, nvim, vim, hx and vi`},
	{"catalogUrl", "Catalog source for update; default: https://gitmoji.dev/api/gitmojis"},
}

var general = []entry{
	{"-h, --help", "Show help; after a command, that command's help"},
	{"-v, --version", "Show the version, target and build revision"},
}

var notes = []string{
	"Flags may come before or after the command; -- ends flags.",
	"Values starting with a dash: --title=-value. Repeated flags: the last value wins.",
}

var files = []entry{
	{"~/.config/cmt/config.json", "Global preferences on Linux ($XDG_CONFIG_HOME when set); ~/Library/Application Support/cmt on macOS, %AppData%\\cmt on Windows"},
	{".cmtrc.json, package.json", "Project preferences, nearest directory first"},
	{"~/.cmt/emojis.json", "Catalog cache written by update"},
	{"$GIT_DIR/cmt-draft.json", "Message kept after a failed commit, until one succeeds"},
}

var exitStatus = []entry{
	{"0", "Success"},
	{"1", "Failure"},
	{"2", "Invalid arguments, preferences or message"},
	{"130", "Cancelled"},
	{"other", "Exit status of a failed Git command"},
}

func table(b *strings.Builder, title string, items []entry) {
	width := 0
	for _, item := range items {
		width = max(width, utf8.RuneCountInString(item.name))
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, item := range items {
		fmt.Fprintf(b, "  %-*s  %s\n", width, item.name, item.text)
	}
}

// Help returns the overview, or the help of a command when name is one.
func Help(name string) string {
	var b strings.Builder
	for _, c := range commands {
		if c.name != name {
			continue
		}
		fmt.Fprintf(&b, "Usage: %s\n\n%s.\n", c.usage, c.summary)
		flags := c.flags
		if c.message {
			flags = append(slices.Clip(messageFlags), flags...)
		}
		if len(flags) > 0 {
			table(&b, "Flags", flags)
		}
		if c.preferences {
			table(&b, "Preferences (--set KEY=JSON)", preferences)
		}
		if c.message {
			table(&b, "Keys in prompts", keys)
		}
		if len(c.notes) > 0 {
			b.WriteString("\n")
			for _, note := range c.notes {
				b.WriteString(note + "\n")
			}
		}
		return b.String()
	}
	b.WriteString("Usage: cmt <command> [flags]\n")
	overview := make([]entry, len(commands))
	for i, c := range commands {
		overview[i] = entry{strings.TrimSuffix(c.name+", "+c.aliases, ", "), c.summary}
	}
	table(&b, "Commands", overview)
	table(&b, "Flags", general)
	b.WriteString("\n")
	for _, note := range notes {
		b.WriteString(note + "\n")
	}
	b.WriteString("Run cmt <command> --help for its flags, keys and preferences, or man cmt.\n")
	return b.String()
}

// HelpFor returns the help that matches parsed options.
func HelpFor(o Options) string {
	if o.Prototype {
		return Help("prototype")
	}
	return Help(o.Command)
}

var roffEscaper = strings.NewReplacer(`\`, `\e`, "-", `\-`, "'", `\(aq`)

func roff(text string) string {
	text = roffEscaper.Replace(text)
	if strings.HasPrefix(text, ".") {
		return `\&` + text
	}
	return text
}

func roffTable(b *strings.Builder, items []entry) {
	for _, item := range items {
		fmt.Fprintf(b, ".TP\n.B %s\n%s\n", roff(item.name), roff(item.text))
	}
}

// Man returns the cmt(1) page, generated from the same entries as Help.
func Man() string {
	var b strings.Builder
	// No hyphenation or justification: commands, paths and URLs must copy intact.
	// groff's man macros restore both on every paragraph from HY and AD.
	b.WriteString(".TH CMT 1 \"\" cmt \"User Commands\"\n.nr HY 0\n.ds AD l\n.nh\n.ad l\n.SH NAME\ncmt \\- prepare Conventional Commits and gitmoji messages\n")
	b.WriteString(".SH SYNOPSIS\n.B cmt\n.I command\n.RI [ flags ]\n")
	b.WriteString(".SH DESCRIPTION\n" + roff("cmt builds commit messages in standard (type(scope): description), emoji, or hybrid mode, and commits the staged changes or reviews them from a Git hook. Normal use is offline.") + "\n")
	b.WriteString(".SH COMMANDS\n")
	for _, c := range commands {
		fmt.Fprintf(&b, ".SS %s\n%s.\n", roff(c.usage), roff(c.summary))
		if c.aliases != "" {
			fmt.Fprintf(&b, ".PP\nAlias: %s\n", roff(c.aliases))
		}
		roffTable(&b, c.flags)
		for _, note := range c.notes {
			fmt.Fprintf(&b, ".PP\n%s\n", roff(note))
		}
	}
	b.WriteString(".SH OPTIONS\nMessage flags for commit, prototype and hook:\n")
	roffTable(&b, messageFlags)
	b.WriteString(".PP\nGeneral flags:\n")
	roffTable(&b, general)
	for _, note := range notes {
		fmt.Fprintf(&b, ".PP\n%s\n", roff(note))
	}
	b.WriteString(".SH KEYS\n")
	roffTable(&b, keys)
	b.WriteString(".SH PREFERENCES\nSave them with config \\-\\-set KEY=JSON, or set them in a project file.\n")
	roffTable(&b, preferences)
	b.WriteString(".SH FILES\n")
	roffTable(&b, files)
	b.WriteString(".SH EXIT STATUS\n")
	roffTable(&b, exitStatus)
	b.WriteString(".SH SEE ALSO\n.BR git (1),\n.BR githooks (5)\n")
	return b.String()
}
