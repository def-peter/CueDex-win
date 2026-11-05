# Windows integration-host research (2026-11-05)

Both decisive questions are now answered from primary sources. **Codex** runs a
`hooks.json` command through `%COMSPEC%` with `/C` and a `raw_arg`-quoted string
(`cmd.exe /C ""<line>""`), so the hook line must be cmd.exe syntax; the file
lives at `<CODEX_HOME>/hooks.json` and each command handler accepts a
Windows-only `commandWindows` line that wins on Windows. **OpenCode** 2.2.5
loads local plugins from `<config root>/plugins` (plural, `XDG_CONFIG_HOME` or
`~/.config`, then `opencode`), where a plugin is `export default { id, setup }`
and `ctx.event.subscribe()` delivers durable `session.execution.*` /
`session.step.*` events whose `id` and `created` give a stable per-completion
identity. A working OpenCode bridge written by the macOS sibling is already on
this machine at `~/.config/opencode/plugins/cuedex.js`, so the Windows port is a
path-and-launcher change, not a new integration. What remains open is narrow:
hook trust enforcement, and whether `Stop` is main-session-only.

## Codex hooks on Windows

1. **Config directory.** `find_codex_home()` returns `CODEX_HOME` when set
   (it must then exist and be a directory, canonicalized, else error) and
   otherwise `~/.codex` without verifying existence (`codex-rs/utils/home-dir/src/lib.rs:5-25`).
   On Windows `~` is `%USERPROFILE%`, so the default is
   `C:\Users\<user>\.codex`. An empty `CODEX_HOME` is ignored
   (`:15-17`).

2. **File and schema.** User hooks are read from `<codex_home>/hooks.json`
   (`codex-rs/hooks/src/engine/discovery.rs:390-431`; the test suite writes
   `home.join("hooks.json")`, `codex-rs/core/tests/suite/hooks.rs:221-225`).
   The file parses as `HooksFile { description?, hooks: HookEventsToml }` with
   `deny_unknown_fields` (`codex-rs/config/src/hook_config.rs:12-19`), so a
   stray top-level key fails the whole file. Event keys are PascalCase — the
   struct renames each field (`PreToolUse`, `PermissionRequest`, `PostToolUse`,
   …, `Stop`; `codex-rs/config/src/hook_config.rs:36-…`, mirrored by the
   `#[serde(rename = "SubagentStart")]` arms in `codex-rs/hooks/src/schema.rs:110-120`).
   A group is `{ "matcher"?: string, "hooks": [...] }` and a command handler is
   `{ "type": "command", "command": string, "commandWindows"?: string,
   "timeout"?: number, "async"?: bool, "statusMessage"?: string }`
   (`codex-rs/config/src/hook_config.rs:158-…`). The shipped test fixture is
   exactly this shape (`codex-rs/hooks/src/engine/mod_tests.rs:1294-1312`).

3. **`commandWindows` wins on Windows.** Discovery replaces the command with
   `command_windows.unwrap_or(command)` under `cfg!(windows)`, then rejects an
   empty result (`codex-rs/hooks/src/engine/discovery.rs:502-512`). A
   Windows-only app can therefore write one entry with both lines and never
   carry a POSIX command.

4. **How a command actually runs (the decisive question).**
   `build_command()` starts `Command::new(default_shell_program(environment))`,
   which is `%COMSPEC%` or `cmd.exe` on Windows (`:499-521`), adds `arg("/C")`
   (`:455-459`), then appends the line with
   `command.raw_arg(format!(r#""{command_line}""#))` (`:463-468`) — i.e.
   `cmd.exe /C ""<command line>""`, with no extra quoting applied by Rust.
   The child gets `CREATE_NO_WINDOW` (`:474-475`), is spawned through
   `JobObject::spawn_background` so the whole tree can be killed
   (`:284`, `:387-411`), and runs with `env_clear()` followed by the session's
   environment snapshot plus the hook's own `env`
   (`:476-495`). Consequence: the line must be **cmd.exe syntax**; PowerShell
   and POSIX quoting are not interpreted. Quoting a single executable path and
   passing arguments is the safe form — exactly what the macOS installer does
   with `shellQuoted(helperPath)` (`CueDex/Services/CodexIntegrationInstaller.swift:166-172`).

5. **Stdin, output, timeout.** `stdin` is piped and receives the hook event
   JSON (`codex-rs/hooks/src/engine/command_runner.rs:314-320`); stdout and
   stderr are piped (`:479-482`) and read by the engine. The deadline covers
   the stdin write as well as the wait (`:332-335`). Timeout defaults to 600s
   and is clamped to at least 1s when unset (`discovery.rs:762`), so a short
   explicit `timeout` is worth setting. Whether a nonzero exit aborts the turn
   was not read out of the engine; treat exit 0 plus no output as the contract.

6. **Payload fields.** The macOS helper parses `hook_event_name`, `session_id`,
   `turn_id`, `cwd`, `transcript_path`, `model`, `permission_mode`,
   `stop_hook_active`, `last_assistant_message` from stdin
   (`CodexIntegrationInstaller.swift:321-330`) and hashes `session_id` +
   `turn_id` into its event identity (`:358-368`). That matches the protocol's
   `sessionID` + stable `eventID` requirement without touching message text.

7. **Event names.** Codex ships `SessionStart`, `SessionEnd`,
   `UserPromptSubmit`, `SubagentStart`, `SubagentStop`, `Stop`, `Interrupt`
   (`codex-rs/hooks/src/lib.rs:29`, wire names lowercased at `:102-110`). The
   existence of a separate `SubagentStop` suggests `Stop` is the main-session
   completion signal, but that is an inference, not a read statement.

8. **Trust.** Each configured handler gets a content-hash identity and a
   `HookTrustStatus` computed against a stored `trusted_hash`
   (`codex-rs/hooks/src/engine/discovery.rs:672-690`, `:793-…`), with
   `enabled`/`trusted_hash` state kept per hook key
   (`codex-rs/config/src/hook_config.rs:29-38`). Whether an untrusted user
   `hooks.json` entry is skipped at execution time is unresolved; assume the
   user may have to approve a newly written hook once.

9. **Local evidence.** No `codex` executable on `PATH`; `%USERPROFILE%\.codex`
   exists with `skills`, `tmp`, and `config.toml`, and no `hooks.json` yet.

## OpenCode plugins on Windows

1. **Version.** `@opencode/cli` 2.2.5, repository `anomalyco/opencode`
   (`…\node_modules\@opencode\cli\package.json`), running from a single
   247 MB Bun-compiled `bin\opencode.exe`.

2. **Config root.** The embedded source computes the base as
   `process.env.XDG_CONFIG_HOME || homedir/.config` and joins the app name onto
   it (`get globalConfigPath()`, bundle offset ≈172_513_222). On Windows the
   default is therefore `%USERPROFILE%\.config\opencode`; no `%APPDATA%`
   fallback appears in that expression.

3. **Plugin directory is `plugins`, plural.** The bundle builds the candidate
   roots and maps each to `path.join(root, "plugins")`
   (bundle offset ≈173_745_040), then lists directory entries from them
   (`rq()`, ≈173_745_220). On this machine
   `C:\Users\22683\.config\opencode\plugins` contains
   `cuedex.js`, `orca-opencode-status.js`, and `orca-opencode-status-tui\tui.js`
   — a live local plugin set, not a package-owned folder. Config-level
   `"plugins": ["oh-my-opencode-slim@3.0.7"]` in `opencode.json` is a separate
   surface: npm package specs, resolved through `node_modules`.

4. **Plugin contract.** A local plugin module default-exports
   `{ id, setup(ctx) }`; `setup` may return a teardown function. This is the
   documented "Promise plugin" contract the shipped bridge relies on
   (`plugins/cuedex.js:62-63`, `:141-186`), with `Plugin.define` in
   `@opencode/plugin` being the identity function.

5. **What `ctx` exposes.** `ctx.app.version`, `ctx.session.get({ sessionID })`,
   `ctx.session.list({ parentID, limit, cursor })`, and
   `ctx.event.subscribe({ signal })` returning an async iterable
   (`plugins/cuedex.js:143-186`). Sessions come back as `{ id, parentID,
   location, time: { busy, idle }, outcome }`; `session.list` returns
   `{ data, cursor: { previous, next } }`, not an array (`:36-56`).

6. **Completion events.** The stream carries `server.connected`,
   `session.execution.started|succeeded|failed|interrupted`, and
   `session.step.started|ended|failed`, each with `data.sessionID`,
   `data.assistantMessageID`, `data.finish`, `created` (epoch ms), `id`
   (`evt_…`), and `durable: { seq, aggregateID, version }`
   (`plugins/cuedex.js:66-125`). All of these literals are present in the 2.2.5
   bundle (18 × `session.execution.succeeded`, 12 × `session.step.ended`), so
   the contract holds for the installed CLI. `session.idle` also exists
   (bundle ≈173_850_396, schema `{ sessionID }`) but is coarser: it does not
   say which message finished.

7. **Completion identity and main-vs-child.** `event.id` is the per-completion
   identity, `event.created` the epoch-ms completion time (`:112-117`); the
   bridge rejects anything not matching `^evt_…$` or older than 60 s, and
   requires the last step to have ended with `stop` or `length` (`:103-110`).
   Root sessions are the ones with `parentID == null`
   (`isCurrentRoot`, `:59-68`); descendants must be enumerated per `parentID`
   and be finished with `outcome` in `succeeded|failed|interrupted` before the
   cue is emitted (`descendantsFinished`, `:25-57`).

8. **Process model and launching.** Plugins run inside the OpenCode process
   (Bun), so the bridge starts the app with `node:child_process`
   (`execFile`, `plugins/cuedex.js:4`, `:176`). On Windows the equivalent is
   `execFile(CueDex.exe, ["--cue"], { windowsHide: true })`.

9. **Event file and gate.** The bridge writes
   `cue-opencode-<sha256(eventID)>.json` via a temporary file plus `rename`
   in the same directory, payload `{ source, eventID, sessionID, completedAt }`
   with `completedAt = event.created / 1000` (`:129-139`), and only when a gate
   file still holds the generation string written at install time (`:146-160`,
   `:163-171`). That is exactly the protocol's metadata-only, atomic,
   dedup-keyed event.

10. **Installer behaviour worth porting.** The macOS installer refuses to
    replace a file that does not start with its ownership line
    (`// CueDex managed OpenCode V2 bridge.`), renders the template by
    substituting JSON-escaped path literals, writes a fresh generation UUID to
    the gate file, gates install on `>= 2.1.1, < 3.0.0`, and on uninstall
    deletes the gate first so already-loaded instances stop emitting
    (`CueDex/Services/OpenCodeIntegrationInstaller.swift:18-135`,
    `CueDex/Resources/cuedex-opencode.js`).

## What this means for CueDex

`PROTOCOL.md` needs no change. Both hosts can satisfy it with metadata-only
payloads, an atomic same-directory rename, a stable `eventID`, a `sessionID`,
epoch-second `completedAt`, and a `CueDex.exe --cue` wake.

**Codex side.** Write `%USERPROFILE%\.codex\hooks.json` (or
`%CODEX_HOME%\hooks.json`), merging into an existing file rather than replacing
it, with a `Stop` group whose handler is
`{ "type": "command", "commandWindows": "\"<abs path to CueDex.exe>\" --cue",
"timeout": 3 }`. Because the line runs through `cmd.exe /C ""…""`, quote the
executable path, use no shell metacharacters, print nothing, and exit 0. If a
stdin-reading helper is preferred over `--cue`, it must still be a cmd.exe
line. Serialize `session_id`/`turn_id` into the event file name and `eventID`
as the mac helper does, and never persist `last_assistant_message`.

**OpenCode side.** Write `%USERPROFILE%\.config\opencode\plugins\cuedex.js`
(honouring `XDG_CONFIG_HOME`), rendered from the same template with the Windows
events directory (`%APPDATA%\CueDex\Events`), gate path, generation UUID and
executable path, and `execFile(exe, ["--cue"], { windowsHide: true })` as the
launcher. Keep the ownership header, the version gate, the 60 s freshness
check, the `step.ended` finish check, and the descendant-drain before emitting.

**Verification.** Both need a Windows end-to-end test that proves: the file
appears, the hook or plugin fires on a real completion, exactly one event per
completion is written, and `CueDex.exe --cue` is started without a console
window.

## Open questions

- Whether Codex executes an untrusted, config-derived `hooks.json` entry or
  holds it behind the `HookTrustStatus` gate until the user approves it.
- Whether Codex `Stop` fires only for the main session and never for a child
  (`SubagentStop` exists, but the dispatch rule was not read).
- Whether a nonzero hook exit code aborts the Codex turn.
- The exact argument `iq()` receives for the global config root in 2.2.5; the
  plural `plugins` folder is settled empirically by the installed working
  plugin, not by reading that one call site.
- Whether OpenCode 2.2.5 watches `plugins/` for changes or only scans at
  startup (affects whether install requires an OpenCode restart).
