# Design: `/join` — 24/7 Voice Channel Presence

> Based on architecture analysis at `docs/architecture-analysis.md` (commit `264c050`)

---

## 1. Overview

`/join` makes the bot stay in a voice channel indefinitely (24/7 mode), even when the playback queue is empty. The bot will idle in the channel and accept new `/play` commands without requiring a rejoin. A companion `/leave` command is introduced to manually disconnect.

---

## 2. Components Reused (unchanged)

| Component | Role in `/join` | File |
|-----------|----------------|------|
| `DiscordSinkProvider` | Manage VC join/leave lifecycle | `internal/discord/voice/sink/provider_discord.go` |
| `DiscordSink` | Stream Opus to Discord | `internal/discord/voice/sink/sink_discord.go` |
| `Player` | Queue + playback engine (modified, see §3) | `pkg/music/player/player.go` |
| `RecoveryStream` | Retryable media opening | `pkg/music/stream/stream.go` |
| `Resolver` | Track resolution | `pkg/music/resolve/resolve.go` |
| `execguard` | Concurrency limiter | `internal/discord/execguard/execguard.go` |
| `reply` | Embed builders | `internal/discord/reply/reply.go` |
| `perm` | Voice channel permission checks | `internal/discord/perm/perm.go` |
| `cmdadapter.Handler` | Slash command interface | `internal/discord/cmdadapter/interfaces.go` |
| `SessionGetter` | Current session indirection | `internal/discord/voice/service.go` |
| `cmdlogger` | Command invocation logging | `internal/discord/cmdlogger/cmdlogger.go` |
| `config` | Env-based configuration (new field added) | `internal/config/config.go` |
| `middleware` | Group access, guild-only, permission checks | `internal/middleware/middleware.go` |

**Remaining untouched**: `pkg/music/stream/`, `pkg/music/parsers/`, `pkg/music/sources/`, `pkg/music/resolve/`, `pkg/music/opus/`, `pkg/music/soundcloudapi/`, `internal/storage/`, `internal/applog/`, `internal/domain/`, `internal/readme/`, `pkg/discordgo-fork-dev/`.

---

## 3. Files That Must Change

| File | Change | Why |
|------|--------|-----|
| `internal/command/music/join/join.go` | **NEW** — `/join` handler | New command |
| `internal/command/music/leave/leave.go` | **NEW** — `/leave` handler | New command |
| `internal/discord/voice/service.go` | Add 24/7 guild set, modify status watcher, add reconnect-targets | Core state change |
| `pkg/music/player/player.go` | Add `StayConnected` option, modify stop-on-empty behavior | Player lifecycle change |
| `pkg/music/player/player_queue.go` | Add `IsStayConnected()`, `SetStayConnected(bool)` | Accessor for 24/7 mode |
| `internal/command/music/stop/stop.go` | Conditional disconnect in 24/7 mode | Behavior change |
| `internal/command/commands.go` | Register `/join` and `/leave` | Command wiring |
| `internal/config/config.go` | Add `DefaultStayConnected` bool env var | Configurability |
| `cmd/discord/main.go` | Wire 24/7 rejoin logic on session ready | Session recovery |

---

## 4. Functions to Modify

### 4.1 `pkg/music/player/player.go` — Player struct

**Add field**:
```go
type Player struct {
    // ... existing fields
    stayConnected bool   // NEW: if true, don't disconnect when queue empties
}
```

**Modify `Stop(disconnect bool)`**:
- When `disconnect=true` AND `stayConnected=true` → disconnect=false, then log warning
- In 24/7 mode, `Stop(true)` is rejected; enforce via command handler calling `SetStayConnected(false)` first

**Add `SetStayConnected(bool)` / `IsStayConnected()`** — simple accessors.

### 4.2 `pkg/music/player/player_queue.go` — Auto-advance goroutine (in `startTrack`)

**Current behavior** (line ~77 of architecture-analysis §7):
```
Queue empty → Stop(true) → ReleaseSink + leave VC
```

**New behavior**:
```
Queue empty AND stayConnected=false → Stop(true) → ReleaseSink + leave VC
Queue empty AND stayConnected=true  → emitStatus(StatusIdle) → keep sink alive
```

Add a new `PlayerStatus` value: `StatusIdle`. The existing `watchPlayerStatus` goroutine (see §11 of analysis) already consumes `PlayerStatus` for async UI updates — it will handle the new status with no structural change.

### 4.3 `internal/discord/voice/service.go` — voice.Service

**Add fields**:
```go
type Service struct {
    // ... existing fields
    stayConnectedGuilds map[string]bool           // guildID → 24/7 enabled
    reconnectTargets    map[string]string          // guildID → channelID (for session restart)
    reconnectMu         sync.Mutex
}
```

**Modify `GetOrCreatePlayer(guildID)`**:
- After creating player, call `p.SetStayConnected(stayConnectedGuilds[guildID])`

**Add `SetStayConnected(guildID string, enabled bool, channelID string)`**:
- Updates `stayConnectedGuilds` map
- If enabled, stores `reconnectTargets[guildID] = channelID`
- If disabled, deletes both entries

**Modify `watchPlayerStatus` goroutine**:
- Handle `StatusIdle`: UpdatePlaybackStatus with an "Idle — waiting for tracks" embed. Do not disconnect.

**Add `RejoinStayConnectedTargets(session)`**:
- Called after session (re-)connect
- Iterates `reconnectTargets`, calls `Sink(reconnectTargets[guildID])` for each
- Updates players' sink provider references if they were invalidated

### 4.4 `internal/command/music/stop/stop.go`

**Current behavior**: Stops playback, disconnects VC.

**New behavior**:
```go
func (h *StopHandler) Run(ctx interface{}) error {
    // ... existing logic
    if player.IsStayConnected() {
        // Clear queue, stop current track, but DO NOT disconnect
        player.Stop(false)
        reply.RespondEmbedEphemeral(/* "Stopped. Bot stays in VC. Use /leave to disconnect." */)
    } else {
        player.Stop(true)
        // ... existing behavior
    }
}
```

### 4.5 `internal/discord/voice/service.go` — `StopAllPlayers()` and `InvalidateAllSinks()`

**`StopAllPlayers()`** (shutdown):
- Force-stop all players regardless of stayConnected
- Send ephemeral status that 24/7 mode ended due to shutdown
- Clear `reconnectTargets` — no rejoin on next startup

**`InvalidateAllSinks()`** (session recovery):
- Invalidates sinks (existing behavior)
- Does NOT clear `reconnectTargets` — `RejoinStayConnectedTargets` will rejoin after session comes up

### 4.6 `cmd/discord/main.go` — onReady handler

After session opens and onReady fires, call:

```go
bot.VoiceService.RejoinStayConnectedTargets(session)
```

This must happen after `dg.Open()` returns and the onReady event is received (see §4 of analysis — `onReady → tracker.MarkReadyNow() + onReady()`).

---

## 5. Functions That Remain Untouched

| Function | File | Reason |
|----------|------|--------|
| `Player.Enqueue()` | `player_queue.go` | No queue logic changes |
| `Player.EnqueueTrackInfo()` | `player_queue.go` | No queue logic changes |
| `Player.PlayNext()` | `player.go` | Changes are in the *post-playback goroutine*, not PlayNext itself |
| `Player.startTrack()` | `player.go` | Track start logic unchanged |
| `Player.runPlayback()` | `player_playback.go` | Streaming loop unchanged |
| `Player.Pause()` / `Resume()` | `player.go` | Still stubs |
| `DiscordSinkProvider.Sink()` | `sink/provider_discord.go` | Join VC logic unchanged |
| `DiscordSinkProvider.ReleaseSink()` | `sink/provider_discord.go` | Leave VC logic unchanged |
| `DiscordSink.Stream()` | `sink/sink_discord.go` | Opus streaming unchanged |
| `RecoveryStream.Open()` | `stream/stream.go` | Media opening unchanged |
| `Watchdog` functions | `watchdog/` | Health detection unchanged |
| `execguard` | `execguard.go` | Concurrency unchanged |
| `cmdlogger` | `cmdlogger.go` | Logging unchanged |
| All middleware | `middleware/middleware.go` | Auth/access unchanged |
| All `pkg/music/parsers/` | — | Media parsing unchanged |
| All `pkg/music/resolve/` | — | Track resolution unchanged |
| All `pkg/music/sources/` | — | Source detection unchanged |
| All `pkg/music/stream/` | — | Media streaming unchanged |
| All `pkg/music/opus/` | — | Opus encoding unchanged |
| `internal/storage/` | — | Persistence unchanged |
| `internal/config/config.go` | — | Adding field only, no logic change |
| `internal/applog/` | — | Logging unchanged |
| `internal/domain/` | — | Types unchanged |

---

## 6. Player Lifecycle Adaptation for 24/7

### Current flow (queue empties):
```
auto-advance goroutine
  → queue empty
  → Stop(true)
    → ReleaseSink  ← LEAVE VOICE CHANNEL
    → emitStatus(StatusStopped)
```

### New flow (24/7 mode, queue empties):
```
auto-advance goroutine
  → queue empty
  → stayConnected==true
  → Stop(false)   ← DO NOT release sink
  → emitStatus(StatusIdle)
  → playable state: yes, next /play will call PlayNext directly
```

The sink stays open. The next `/play` command:
1. Enqueues track
2. Calls `PlayNext(target)` 
3. `PlayNext` calls `Stop(false)` if playing (sink already exists, no rejoin needed)
4. `startTrack` → `sink.Sink(target)` — this is a no-op if already connected to the same channel
5. Playback begins

**Key invariant**: In 24/7 mode, the `DiscordSinkProvider` must be able to return an existing sink without re-joining. Currently `Sink(target)` always calls `dg.ChannelVoiceJoin()`. This needs a short-circuit when already connected to `target`.

### `DiscordSinkProvider.Sink(target)` — modification needed

```go
func (p *DiscordSinkProvider) Sink(target string) (AudioSink, error) {
    p.mu.Lock()
    if p.vc != nil && p.currentChannelID == target {
        p.mu.Unlock()
        return p.sink, nil  // Already connected to this channel — no-op
    }
    p.mu.Unlock()
    // ... existing join logic
}
```

Add `currentChannelID` field to `DiscordSinkProvider` to track which channel we're in.

---

## 7. `/stop` Behavior in 24/7 Mode

| Scenario | Action | User sees |
|----------|--------|-----------|
| `/stop` while playing, 24/7 on | Stop track, clear queue, keep VC | "Stopped. Bot stays in channel. Use /leave to disconnect." |
| `/stop` while playing, 24/7 off | Stop track, clear queue, leave VC | "Stopped and disconnected." (current) |
| `/stop` while idle, 24/7 on | Clear queue (already empty), keep VC | "Queue already empty. Bot is staying in channel." |
| `/stop` while idle, 24/7 off | No-op | "Not playing anything." (current) |

---

## 8. `/leave` Command

New slash command, no arguments.

**Behavior**:
1. If not connected to VC → reply ephemeral "Not in a voice channel."
2. Call `Service.SetStayConnected(guildID, false, "")` — disables 24/7
3. Call `player.Stop(true)` — clears queue, releases sink, leaves VC
4. Reply ephemeral "Disconnected. Use /join to bring me back."

**Permissions**: Same as `/stop` — `VoiceChannelConnect` + `VoiceChannelSpeak`.

---

## 9. Automatic Reconnect and 24/7 Mode

### Session restart flow (from §10 of analysis):

```
Unhealthy detected → disconnected channel closed → RunSession returns
  → main loop: delay → go bot.RunSession()
    → New discordgo.Session created
    → dg.Open()
    → onReady fires → RejoinStayConnectedTargets() called
```

**`RejoinStayConnectedTargets(session)`**:
```go
func (s *Service) RejoinStayConnectedTargets(session *discordgo.Session) {
    s.reconnectMu.Lock()
    defer s.reconnectMu.Unlock()
    for guildID, channelID := range s.reconnectTargets {
        // Get or create player for this guild
        player := s.GetOrCreatePlayer(guildID)
        // Get sink provider
        provider := s.sinkProviders[guildID]
        // Rejoin VC
        sink, err := provider.Sink(channelID)
        if err != nil {
            s.appLog.Warn().Err(err).Str("guild", guildID).Msg("Rejoin failed during session recovery")
            // After N failures, clear reconnect target to prevent infinite loop
            delete(s.reconnectTargets, guildID)
            continue
        }
        // Update player's sink provider reference if needed
        player.SetSinkProvider(provider)
        // Set stayConnected
        player.SetStayConnected(true)
        // Emit idle status
        s.watchPlayerStatus(guildID, player)
    }
}
```

### Session restart edge cases:

| Event | 24/7 behavior |
|-------|---------------|
| Session restart (unhealthy) | `reconnectTargets` preserved. Rejoin after onReady. |
| Bot process restart | `reconnectTargets` is in-memory only → lost. 24/7 state not persisted to disk (intentional — simpler and session restart is rare). |
| Bot kicked from guild | `onGuildDelete` handler: clean up `reconnectTargets[guildID]`, `stayConnectedGuilds[guildID]`, `sinkProviders[guildID]`, `players[guildID]`. |
| Discor d voice channel deleted | `Sink(channelID)` will fail → `RejoinStayConnectedTargets` logs error and clears target. |

---

## 10. Session Restart Interaction with Existing Voice Connections

### Current flow (from §10 of analysis):
- `InvalidateAllSinks()` is called on unhealthy detection
- This invalidates the existing `DiscordSinkProvider.vc` connections
- On new session, `SessionGetter` returns the new session
- When a new `/play` comes in, `Sink(target)` creates a fresh connection

### With 24/7:
- `InvalidateAllSinks()` still disconnects old VC
- But `reconnectTargets` remembers guild→channel mapping
- After new session opens, `RejoinStayConnectedTargets` joins fresh VCs
- `SessionGetter` indirection ensures all sinks use the new session

**Race**: If a `/play` command arrives between session restart and rejoin completion:
- Player has `stayConnected=true` but sink is invalid
- `PlayNext` → `startTrack` → `sink.Sink(target)` → rejoin happens naturally
- No deadlock — the Sink() call functions as a rejoin even outside the rejoin loop

**Guard**: During rejoin, acquire a `reconnectMu` to prevent concurrent attempts.

---

## 11. Race Conditions Analysis

| # | Race | Severity | Mitigation |
|---|------|----------|------------|
| 1 | `/play` + `/stop` simultaneously while 24/7 is on | Low | `execguard` serializes commands per guild (semaphore). |
| 2 | `/leave` + `/play` simultaneously | Low | execguard serializes. If `/play` acquires first, `/leave` waits. |
| 3 | Auto-advance emits StatusIdle while `/join` is being processed | Low | Status channel is consumed in `watchPlayerStatus` goroutine. No shared mutable state with command handler — commands use synchronous responses, watcher handles async only. |
| 4 | Session restart while 24/7 active | Medium | Rejoin loop runs after onReady. During gap, `/play` implicitly rejoins via `Sink()`. No stale state because old sink was invalidated. |
| 5 | Bot in 24/7 mode on Guild A, user calls `/join` on Guild B | None | Different guildIDs → different players, different providers. No shared state. |
| 6 | Two session restarts in rapid succession | Low | `reconnectMu` prevents concurrent rejoin. Second restart's rejoin runs after first completes. |
| 7 | User calls `/leave` while rejoin is in progress | Medium | `reconnectMu` acquired by rejoin. `/leave` calls `SetStayConnected(false)` which deletes `reconnectTargets`. After rejoin completes, `/leave` proceeds to `Stop(true)`. Order: rejoin completes, then leave disconnects. No leak. |
| 8 | `stayConnectedGuilds` and `reconnectTargets` maps become inconsistent | Medium | Single `SetStayConnected(guildID, enabled, channelID)` function updates both maps atomically under `reconnectMu`. |

---

## 12. New Status: `StatusIdle`

Add to `pkg/music/player/player_status.go`:
```go
const (
    StatusPlaying PlayerStatus = iota
    StatusStopped
    StatusError
    StatusIdle                         // NEW
)
```

The `watchPlayerStatus` goroutine in `internal/discord/voice/service.go` will handle `StatusIdle`:
```go
case player.StatusIdle:
    embed := reply.NewIdleEmbed()  // "🔊 Idle — waiting for tracks"
    s.UpdatePlaybackStatus(session, nil, guildID, embed)
```

---

## 13. Config Additions

In `internal/config/config.go`:
```go
DefaultStayConnected bool `env:"DEFAULT_STAY_CONNECTED" envDefault:"false"`
```

When `true`, calling `/play` in a guild without an active player automatically enables 24/7 mode for that guild (equivalent to implicit `/join`). When `false` (default), 24/7 must be explicitly enabled via `/join`.

---

## 14. Implementation Plan

### Phase 1: Foundation (low risk, no behavior change)

1. **Add `StayConnected` to Player** (`pkg/music/player/player.go`):
   - Field + accessors
   - No behavioral change yet

2. **Add `currentChannelID` to DiscordSinkProvider** (`sink/provider_discord.go`):
   - Set after successful `dg.ChannelVoiceJoin`
   - Add early-return in `Sink(target)` when already connected to same target
   - All tests still pass

3. **Add `stayConnectedGuilds` + `reconnectTargets` to Service** (`internal/discord/voice/service.go`):
   - Maps + mutex
   - `SetStayConnected()` method
   - No behavioral change (maps are empty by default)

### Phase 2: Core behavior change (moderate risk)

4. **Modify auto-advance in `startTrack()`** (`pkg/music/player/player_queue.go`):
   - When queue empty and `stayConnected==true`: emit `StatusIdle` instead of calling `Stop(true)`
   - When queue empty and `stayConnected==false`: unchanged

5. **Add `StatusIdle` constant** (`pkg/music/player/player_status.go`):
   - Handle in `watchPlayerStatus`

6. **Modify `/stop` command** (`internal/command/music/stop/stop.go`):
   - Check `player.IsStayConnected()`
   - Conditional disconnect text

### Phase 3: New commands (low risk)

7. **Create `/join` handler** (`internal/command/music/join/join.go`):
   - Implements `cmdadapter.Handler`
   - Checks permissions via `perm`
   - Calls `Service.SetStayConnected(guildID, true, channelID)`
   - Calls `Service.GetOrCreatePlayer(guildID)`
   - Joins VC via sink provider
   - Replies embed "🔊 I'll stay in the channel. Use /leave to disconnect."

8. **Create `/leave` handler** (`internal/command/music/leave/leave.go`):
   - Calls `SetStayConnected(guildID, false, "")`
   - Calls `player.Stop(true)`
   - Replies embed "Disconnected."

9. **Register both commands** in `internal/command/commands.go` + `cmd/discord/main.go`

### Phase 4: Session recovery (moderate risk)

10. **Implement `RejoinStayConnectedTargets`** (`internal/discord/voice/service.go`):
    - Join VCs from `reconnectTargets`
    - Handle partial failures gracefully

11. **Wire into onReady** (`cmd/discord/main.go`):
    - Call after session opens

12. **Add `onGuildDelete` cleanup** (`internal/discord/handlers.go`):
    - Remove guild from `stayConnectedGuilds` and `reconnectTargets`
    - Removes player and sink provider

### Phase 5: Config and polish (low risk)

13. **Add `DEFAULT_STAY_CONNECTED` env var** (`internal/config/config.go`)

14. **Integration test**: simulate queue-empty in 24/7 mode, verify VC stays connected

---

## 15. Sequence Diagram

```
User                    Discord            /join handler          voice.Service            Player           SinkProvider
 │                        │                     │                     │                      │                  │
 │  /join (in VC)         │                     │                     │                      │                  │
 │───────────────────────►│────────────────────►│                     │                      │                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │ SetStayConnected(guildID, true, channelID)  │                  │
 │                        │                     │────────────────────►│                      │                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │ GetOrCreatePlayer()  │                      │                  │
 │                        │                     │────────────────────►│                      │                  │
 │                        │                     │                     │ SetStayConnected(true) │                  │
 │                        │                     │                     │──────────────────────►│                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │                     │ Sink(channelID)      │                  │
 │                        │                     │                     │─────────────────────────────────────────►│
 │                        │                     │                     │                      │                  │
 │                        │                     │                     │◄───────── DiscordSink ──────────────────│
 │                        │                     │                     │                      │                  │
 │                        │                     │◄── ok ──────────────│                      │                  │
 │                        │◄── embed ───────────│                     │                      │                  │
 │◄── "I'll stay in ──────│                     │                     │                      │                  │
 │     the channel"       │                     │                     │                      │                  │
 │                        │                     │                     │                      │                  │
 │  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─│
 │                        │                     │                     │                      │                  │
 │  /play (later)         │                     │                     │                      │                  │
 │───────────────────────►│────────────────────►│  (play handler)      │                      │                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │ PlayNext(target)    │                      │                  │
 │                        │                     │────────────────────►│─────────────────────►│                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │                     │ Sink(target)         │                  │
 │                        │                     │                     │ ── already connected ─►│ (no-op)         │
 │                        │                     │                     │                      │                  │
 │                        │                     │                     │ startTrack()         │                  │
 │                        │                     │                     │──────────────────────►│                  │
 │                        │                     │                     │                      │  ▶ streaming     │
 │                        │                     │                     │                      │                  │
 │  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─│
 │                        │                     │                     │                      │                  │
 │  Queue empty           │                     │                     │                      │                  │
 │                        │                     │                     │ auto-advance fires    │                  │
 │                        │                     │                     │ queue empty           │                  │
 │                        │                     │                     │ stayConnected==true   │                  │
 │                        │                     │                     │──────────────────────►│                  │
 │                        │                     │                     │ Stop(false)           │                  │
 │                        │                     │                     │──────────────────────►│                  │
 │                        │                     │                     │ emitStatus(StatusIdle) │                  │
 │                        │                     │                     │◄──────────────────────│                  │
 │                        │                     │                     │                      │                  │
 │                        │                     │  (watchPlayerStatus) │                      │                  │
 │                        │                     │◄──── StatusIdle ────│                      │                  │
 │                        │◄── "Idle" embed ────│                     │                      │                  │
 │◄── "🔊 Idle..." ───────│                     │                     │                      │                  │
```

---

## 16. Risks

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|------------|--------|------------|
| 1 | Bot stays in VC forever if `/leave` is never called | Low (user error) | Low (wastes 1 voice connection per guild) | Idle timeout config: `VOICE_IDLE_TIMEOUT` (optional, default 0 = forever) that disconnects after N minutes of inactivity. Add after Phase 5. |
| 2 | Discord voice channel deleted while bot is idle | Low | Low | Next `/play` fails on `Sink()` with error → user retries with `/join` |
| 3 | Bot reaches Discord voice connection limit (250 per bot) | Very low (single-guild scenarios) | Medium | No mitigation needed for initial implementation. Add max-guilds limit later. |
| 4 | Session restart causes brief audio gap for all 24/7 guilds | Medium | Medium | Rejoin happens after onReady (~2-5s). Acceptable. |
| 5 | `reconnectTargets` grows unboundedly | Low | Low | Cleaned up on guild leave and `/leave`. Bound with max (e.g., 250). |
| 6 | `/play` before rejoin completes during session restart | Medium | Low | `Sink()` handles rejoin naturally. Slightly delayed start. |
| 7 | 24/7 state lost on bot restart | High | Low | Acceptable — user calls `/join` again. Not worth disk persistence for rare restarts. |
| 8 | Execguard timeout while waiting for rejoin | Low | Low | Rejoin should complete within timeout (15s join timeout). If not, command fails gracefully. |

---

## 17. Rollback Strategy

### Per-deployment rollback:

1. **Revert code**: `git revert <merge-commit>` for the PR.
2. **Database/data**: No schema changes — no data migration needed.
3. **Config**: Remove `DEFAULT_STAY_CONNECTED` env var if set. No breakage if left (ignored by old binary).
4. **Deploy**: Rebuild and restart.

### Feature flag:

Ship behind `DEFAULT_STAY_CONNECTED` env var (default `false`). Even if code is deployed, 24/7 mode is opt-in. To disable entirely:

```bash
DISABLE_JOIN_COMMAND=true  # New config var
```

If set, `/join` and `/leave` are not registered. `/stop` reverts to original behavior.

### Partial rollback (if only `/leave` has issues):

- Remove `/leave` from command registration
- `/stop` with forced `disconnect=true` can serve as a manual workaround (`player.SetStayConnected(false)` from admin command `/maintenance`)

---

*Design based on melodix architecture at commit `264c050`.*