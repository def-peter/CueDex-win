# Agent instructions

Read `HANDOFF.md` before changing anything. It records the decisions that must
not be reopened (Go + MyGo, Windows 11 only, two separate projects), the
platform-parity rules, the reference examples, and the spike that comes first.

`PROTOCOL.md` is the only contract shared with the macOS app.

Verify with `go vet ./...`, `gofmt -l .`, and `go test ./...` before committing.

## Agent skills

### Issue tracker

Issues and specs live as GitHub issues, through the `gh` CLI. See
`docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, each label string equal to its name. See
`docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` at the root, decisions in `docs/adr/`. See
`docs/agents/domain.md`.
