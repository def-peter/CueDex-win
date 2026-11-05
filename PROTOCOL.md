# CueDex event protocol

The macOS app in `def-peter/CueDex` and this Windows app are maintained as two
separate projects. They share no code. This file is the only shared contract:
each app's integration helper must produce events that the other side's rules
also describe, so the two implementations stay behaviourally identical.

Prompt and response content is never written. Every field below is metadata.

## Event directory

| Platform | Directory |
| --- | --- |
| macOS | `~/Library/Application Support/CueDex/Events` |
| Windows | `%APPDATA%\CueDex\Events` |

Created with owner-only permissions where the platform has them. The app
watches the directory and consumes new files without polling.

## Files

A producer writes one JSON file per completion, then wakes the app.

| Producer | File name |
| --- | --- |
| Codex `Stop` hook | `turn-complete.<pid>` |
| OpenCode plugin | `cue-<source>-<sha256hex of eventID>.json` |

Write to a temporary name in the same directory and rename into place, so the
app never reads a partial file. A file larger than 16 KiB is ignored. Files are
deleted after they are read.

## Payload

```json
{
  "source": "codex",
  "eventID": "9f2c…",
  "sessionID": "…",
  "completedAt": 1793491200
}
```

- `source` — `codex` or `opencode`.
- `eventID` — non-empty, at most 512 UTF-8 bytes. Stable for one completion.
- `sessionID` — non-empty, at most 512 UTF-8 bytes. `legacy` when the producer
  has no session identity.
- `completedAt` — seconds since the Unix epoch, integer or fractional.

## Acceptance rules

1. **Encoding** — decode with seconds-since-epoch semantics. Reject a payload
   whose `eventID` or `sessionID` is empty or longer than 512 bytes.
2. **Freshness** — accept only when `0 <= now - completedAt < 60` seconds.
3. **Dedup** — identity is `"<source>:<eventID>"`. Remember identities for 60
   seconds and drop repeats.
4. **Queueing** — keep at most one pending cue per source; a newer
   `completedAt` replaces the pending one for that source.
5. **Legacy** — an empty `turn-complete.*` file is a valid Codex completion.
   Synthesize an event from the file name and its modification time.

## Waking the app

The producer must make a running app cue without the user's intervention.

| Platform | Mechanism |
| --- | --- |
| macOS | `open -gj <CueDex.app>` |
| Windows | start `CueDex.exe --cue`; single-instance forwards the argument to the running app |

Starting the app when it is not running is acceptable; the event is fresh for
60 seconds, so it still cues on arrival.
