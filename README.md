# CueDex for Windows

Windows 11 build of [CueDex](https://github.com/def-peter/CueDex), the attention
router that cues you when a coding assistant finishes a response.

This repository is a separate implementation from the macOS app. The two share
no source code; the only shared contract is [PROTOCOL.md](PROTOCOL.md), which
pins the event format both integration helpers must produce.

Built with [MyGo](https://mygo.egoist.dev) — native UI drawn on Direct3D 11, no
webview, so the app stays cheap while it sits in the notification area.

## Install and use

Release binaries support **Windows 11 x64**. Download from
[GitHub Releases](https://github.com/def-peter/CueDex-win/releases/latest):

- `cuedex-setup-0.2.0-amd64.exe`: NSIS installer.
- `CueDex.exe`: standalone app; keep it in a permanent location before enabling
  integrations.
- `SHA256SUMS.txt`: checksums for both downloads.

The binaries are unsigned. Windows SmartScreen may warn about an unknown
publisher. Verify the download source and checksum before choosing to run it;
do not disable Windows security protections.

Launch CueDex, then enable Codex and/or OpenCode under **Integrations** in its
settings. Restart OpenCode after enabling its plugin. OpenCode stable versions
from 2.1.1 up to (but not including) 3.0.0 are supported; 2.2.5 was tested.
Complete a main-session response to trigger a cue. Closing settings leaves the
app running in the notification area; use its tray menu to reopen settings or
quit. Preferences are stored in `%APPDATA%\CueDex\preferences.json`.

Integrations store the app's absolute path. Before moving the executable or
switching from a development copy to the installer, disable the integrations
and quit the old instance, then launch the new copy and enable them again.

### Known limits in v0.2.0

- Codex's Stop hook command was tested with event payloads, but invocation by a
  real Codex CLI on Windows still needs verification.
- Animation CPU measured 5–7% of one core and RSS grew about 11 MB on the test
  machine, above the original targets of 3% and 10 MB.
- ARM64 artifacts and automatic updates are not included in this release.

## Development

Requires Go 1.27.3 or later.

```powershell
go tool mygo dev     # run with live reload
go test ./...        # view tests run without a window
go tool mygo build   # package the app into build\
```

`go tool mygo build` writes the icon and manifest into the executable. A plain
`go build` produces a binary with no icon resource, so Explorer, the Start menu
and a pinned shortcut show the generic Windows placeholder instead of CueDex's
bell; use the packaged build to check anything outside the running window.

## Scope

Windows 11 only. macOS keeps its own repo and its own release pipeline.

| Capability | Windows implementation |
| --- | --- |
| Tray, settings | MyGo native UI |
| Screen-edge glow | Win32 layered overlay windows, one set per display |
| Sound cues | Bundled WAV files, played at system volume |
| Speech cues | SAPI `SpVoice` with installed voice enumeration |
| Start at login | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` |
| Codex integration | `CueDex.exe --hook codex` registered as a `Stop` hook command |
| OpenCode integration | managed global plugin; tracks execution and step events, applies the rules in `PROTOCOL.md` |

## License

MIT.
