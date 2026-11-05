// CueDex managed OpenCode V2 bridge. Only metadata is written to disk.
import { mkdir, readFile, writeFile, rename, unlink } from "node:fs/promises";
import { createHash, randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import path from "node:path";

export function supportsVersion(version) {
    if (typeof version !== "string") return false;
    const match = /^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.exec(version);
    if (!match || match[0] !== version) return false;
    const [major, minor, patch] = match.slice(1, 4).map(Number);
    return [major, minor, patch].every(Number.isSafeInteger)
        && major === 2 && (minor > 1 || (minor === 1 && patch >= 1));
}

const isRecord = value => value !== null && typeof value === "object" && !Array.isArray(value);
const isTime = value => Number.isFinite(value) && value >= 0;

async function descendantsFinished(session, rootID) {
    if (typeof session?.list !== "function") return false;
    const seen = new Set([rootID]);
    const parents = [rootID];
    let pages = 0;
    for (const parentID of parents) {
        const cursors = new Set();
        let cursor;
        do {
            if (++pages > 256) return false;
            const page = await session.list({ parentID, limit: 100, ...(cursor === undefined ? {} : { cursor }) });
            // V2 returns SessionsResponse directly, not an array or HTTP envelope.
            if (!isRecord(page) || !Array.isArray(page.data) || !isRecord(page.cursor)) return false;
            const { previous, next } = page.cursor;
            if ([previous, next].some(value => value != null && (typeof value !== "string" || !value))) return false;
            for (const child of page.data) {
                if (!isRecord(child) || typeof child.id !== "string" || !child.id || child.parentID !== parentID
                    || seen.has(child.id) || seen.size >= 256 || !isRecord(child.time)
                    || !isTime(child.time.busy) || !isTime(child.time.idle) || child.time.idle < child.time.busy
                    || !["succeeded", "failed", "interrupted"].includes(child.outcome)) return false;
                seen.add(child.id);
                parents.push(child.id);
            }
            if (next == null) break;
            if (cursors.has(next)) return false;
            cursors.add(next);
            cursor = next;
        } while (true);
    }
    // These timestamps only establish known execution endings, not scheduler wake consumption.
    return true;
}

function isCurrentRoot(info, sessionID, created) {
    if (!isRecord(info) || info.id !== sessionID || info.parentID != null || !info.location?.directory) return false;
    // A successful event remains sufficient evidence for older hosts without root execution times.
    if (info.time === undefined) return true;
    if (!isRecord(info.time)) return false;
    const { busy, idle } = info.time;
    return [busy, idle].every(value => value === undefined || (isTime(value) && value <= created))
        && !(busy !== undefined && idle !== undefined && idle < busy);
}

// Kept dependency-free: Plugin.define in @opencode/plugin@2.1.1 is the identity
// function. This is its documented { id, setup } Promise plugin contract.
export function createCompletionTracker({ session, emit, enabled, now = Date.now }) {
    const executions = new Map();
    const reset = () => executions.clear();
    async function accept(event) {
        if (event.type === "server.connected") { reset(); return; }
        const sessionID = event.data?.sessionID;
        const sequence = event.durable?.seq;
        if (typeof sessionID !== "string" || !sessionID || event.durable?.aggregateID !== sessionID
            || !Number.isSafeInteger(sequence) || sequence < 0 || event.durable?.version !== 1) return;
        if (event.type === "session.execution.started") {
            const previous = executions.get(sessionID);
            if (previous && sequence <= previous.seq) return;
            // Bound metadata retained by long-running, globally loaded instances.
            if (executions.size >= 1024) executions.delete(executions.keys().next().value);
            executions.set(sessionID, { seq: sequence, message: null, ended: false });
            return;
        }
        const state = executions.get(sessionID);
        if (!state || sequence <= state.seq) return;
        state.seq = sequence;
        switch (event.type) {
        case "session.step.started":
            state.message = event.data.assistantMessageID;
            state.ended = false;
            break;
        case "session.step.failed":
            state.ended = false;
            break;
        case "session.step.ended":
            state.ended = typeof state.message === "string"
                && state.message === event.data.assistantMessageID
                && ["stop", "length"].includes(event.data.finish);
            break;
        case "session.execution.failed":
        case "session.execution.interrupted":
            executions.delete(sessionID);
            break;
        case "session.execution.succeeded": {
            if (state.completed) return;
            state.completed = true;
            // Keep this identity until the query ends; reset or this root's next start invalidates it.
            const current = () => executions.get(sessionID) === state && state.seq === sequence;
            const fresh = () => {
                const age = now() - event.created;
                return Number.isFinite(age) && age >= 0 && age < 60_000;
            };
            try {
                if (!state.ended || typeof event.id !== "string" || !/^evt_[a-zA-Z0-9_-]+$/.test(event.id)
                    || !fresh() || !await enabled() || !current()) return;
                // The Promise plugin's session.get returns Session.Info directly.
                if (!isCurrentRoot(await session.get({ sessionID }), sessionID, event.created) || !current()) return;
                if (!await descendantsFinished(session, sessionID) || !current()) return;
                // Catch a root that resumed while its descendants were being inspected.
                if (!isCurrentRoot(await session.get({ sessionID }), sessionID, event.created)) return;
                if (!await enabled() || !current() || !fresh()) return;
                await emit({ source: "opencode", eventID: event.id, sessionID, completedAt: event.created / 1000 });
            } finally {
                if (executions.get(sessionID) === state) executions.delete(sessionID);
            }
            break;
        }
        }
    }
    return { accept, reset };
}

export async function writeCompletion(event, { directory, enabled, launch }) {
    await mkdir(directory, { recursive: true, mode: 0o700 });
    const name = createHash("sha256").update(event.eventID).digest("hex");
    const temporary = path.join(directory, `.opencode-${randomUUID()}.tmp`);
    try {
        await writeFile(temporary, JSON.stringify(event), { mode: 0o600, flag: "wx" });
        if (!await enabled()) return;
        await rename(temporary, path.join(directory, `cue-opencode-${name}.json`));
        launch();
    } finally { await unlink(temporary).catch(() => {}); }
}

export default {
    id: "cuedex.attention",
    setup(ctx) {
        if (!supportsVersion(ctx?.app?.version)
            || typeof ctx?.session?.get !== "function"
            || typeof ctx?.session?.list !== "function"
            || typeof ctx?.event?.subscribe !== "function") return;
        const eventsDirectory = "__CUEDEX_EVENTS__";
        const gate = "__CUEDEX_GATE__";
        const generation = "__CUEDEX_GENERATION__";
        const appPath = "__CUEDEX_APP__";
        const controller = new AbortController();
        const enabled = async () => {
            try { return !controller.signal.aborted && (await readFile(gate, "utf8")) === generation; }
            catch { return false; }
        };
        const tracker = createCompletionTracker({
            session: ctx.session,
            enabled,
            emit: event => writeCompletion(event, {
                directory: eventsDirectory, enabled,
                // The Windows app is one executable, not a bundle: run it and
                // let the single-instance lock hand --cue to the running copy.
                launch: () => execFile(appPath, ["--cue"], { windowsHide: true }, () => {}),
            }),
        });
        let retry;
        let resolveRetry;
        void (async () => {
            while (!controller.signal.aborted) {
                tracker.reset();
                try {
                    for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
                        if (controller.signal.aborted) break;
                        try { await tracker.accept(event); }
                        catch { tracker.reset(); } // Never log raw events or private errors.
                    }
                } catch { tracker.reset(); }
                if (controller.signal.aborted) break;
                await new Promise(resolve => { resolveRetry = resolve; retry = setTimeout(resolve, 1000); });
            }
        })();
        return () => {
            controller.abort();
            clearTimeout(retry);
            resolveRetry?.();
            tracker.reset();
        };
    },
};
