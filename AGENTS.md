# Agent instructions

Read `HANDOFF.md` before changing anything. It records the decisions that must
not be reopened (Go + MyGo, Windows 11 only, two separate projects), the
platform-parity rules, the reference examples, and the spike that comes first.

`PROTOCOL.md` is the only contract shared with the macOS app.

Verify with `go vet ./...`, `gofmt -l .`, and `go test ./...` before committing.
