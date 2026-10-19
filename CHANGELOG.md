# Changelog

All notable changes to cmt are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and cmt follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html): from 1.0.0 on, the
commands, flags, preferences, message formats and exit codes are its public interface.

## [1.0.0] - 2026-10-19

First stable release. cmt is a native Go rewrite of gitmoji-cli that prepares
Conventional Commits, gitmoji and hybrid messages. It is a single executable that
works offline and needs only Git: no Node, npm or npx.

### Added

**Messages**

- Three modes: `standard` (`feat(api): add search`, the default), `emoji`
  (`✨ (api): Add search`) and `hybrid` (`feat(api): ✨ add search`).
- The 11 Conventional Commits types: `feat`, `fix`, `docs`, `refactor`, `test`,
  `chore`, `perf`, `style`, `build`, `ci` and `revert`, each with a description.
- Breaking changes as `type!:`, with `--breaking` or the `!` key in the type selector.
- Title limit of 100 characters, counted in UTF-16 units (an emoji such as 🐛 counts 2),
  with a warning above 72 and a live `used/available` counter while typing.

**Prompts**

- Inline prompts that stay in the scrollback: no full-screen interface.
- Selectors filter as you type. The emoji search matches names, codes and
  descriptions, tolerates abbreviations and simple typos, and puts exact matches first.
- Scope selector that suggests the modules of the staged files, your saved
  `customScopes` and the main folders of the repository, plus "Custom scope…" to
  write one. A project list in `scopePrompt` is strict.
- Ctrl+O in the body prompt writes the body in an editor: the `editor` preference,
  or else the first installed of nano, micro, nvim, vim, hx and vi.
- A preview and a confirmation before anything reaches Git.

**Git**

- `cmt commit` commits only what is already staged, including partial staging.
  cmt never runs `git add`.
- `cmt init` installs a managed `prepare-commit-msg` hook, so plain `git commit`
  opens the prompts. `cmt remove` deletes only a hook that cmt recognizes as its own.
- The hook keeps the body, trailers such as `Signed-off-by` and Git's comments.
  It leaves the message untouched without a terminal, during a rebase, and for
  amend, merge, `-c` and `-C` sources.
- GPG signing, identity and Git's own hooks keep working; Git's exit status is
  passed through.
- Message recovery: when the commit fails after you confirm (a hook, the
  signature…), the next `cmt commit` or `git commit` offers `Reuse it? [Y/n]` and
  fills every prompt. The draft lives in the repository's Git directory with
  owner-only permissions and expires when HEAD moves.

**Configuration and catalog**

- Global preferences plus per-project overrides from `.cmtrc.json` or the `cmt` key
  of `package.json`, nearest directory first. `cmt config` edits them;
  `--show`, `--set KEY=JSON` and `--import FILE` work without a terminal.
- Offline catalog of 75 gitmojis embedded in the executable. `list`, `search`,
  and `update`, which is the only command that uses the network.

**Help and distribution**

- `cmt --help`, help for each command (`cmt commit --help`) and a `cmt(1)` man page.
- Packages with SHA256 checksums, build metadata and the license notices of every
  linked module.
- CI runs the tests natively on Linux amd64, Linux arm64 and macOS arm64.

### Changed from gitmoji-cli 9.7.0

- `standard` is the default mode. Imported gitmoji-cli profiles keep `emoji`.
- Nothing is fetched automatically: the catalog is embedded, and there is no update
  notifier.
- `autoAdd` is retired: cmt never stages files. Old profiles that set it still load.
- An unknown command is a usage error (exit status 2) instead of printing the help.
- `init` and `remove` check that a hook belongs to cmt before writing or deleting it.
- The hook keeps the whole received message, not just its first lines.
- Preferences are imported only on request (`cmt config --import FILE`); the original
  profile is never modified.
- The search ranking is cmt's own and does not reproduce Fuse.js.

### Known limitations

- Windows and macOS on Intel are built but not tested natively, so they are not
  published yet. On Windows, `cmt init` refuses to install the hook.
- The macOS executable is not signed or notarized. If macOS blocks it, run
  `xattr -d com.apple.quarantine cmt`.
- The body editor ignores `$EDITOR` and `core.editor`; set it with
  `cmt config --set 'editor="nvim"'`.

## [0.1.0-rc.1] - 2026-10-10

First release candidate of the native rewrite, tagged before the project was
renamed to cmt. It was never published as a release.

[1.0.0]: https://github.com/Snah-s/cmt/releases/tag/v1.0.0
[0.1.0-rc.1]: https://github.com/Snah-s/cmt/tree/v0.1.0-rc.1
