# CueDex for Windows

Windows 11 build of [CueDex](https://github.com/def-peter/CueDex), the attention
router that cues you when a coding assistant finishes a response.

This repository is a separate implementation from the macOS app. The two share
no source code; the only shared contract is [PROTOCOL.md](PROTOCOL.md), which
pins the event format both integration helpers must produce.

Built with [MyGo](https://mygo.egoist.dev) — native UI drawn on Direct3D 11, no
webview, so the app stays cheap while it sits in the notification area.

## Development

Requires Go 1.27.3 or later.

```powershell
go tool mygo dev     # run with live reload
go test ./...        # view tests run without a window
go tool mygo build   # package the app into build\
```

## Scope

Windows 11 only. macOS keeps its own repo and its own release pipeline.

| Capability | Windows implementation |
| --- | --- |
| Tray, settings | MyGo native UI |
| Screen-edge glow | Win32 layered overlay windows, one set per display |
| Sound cues | Bundled WAV files, played at system volume |
| Speech cues | SAPI `SpVoice` with installed voice enumeration |
| Start at login | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` |
| Codex integration | `cuedex-notify.exe` registered as a `Stop` hook command |
| OpenCode integration | managed global plugin, same event tracking as macOS |

## License

MIT, as the macOS app.
