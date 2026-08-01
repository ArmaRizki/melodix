# Melodix — Complete Repository Analysis

> Generated from source at commit `264c050`  
> **Repository root**: `c:/git/melodix`  
> **Package**: `github.com/keshon/melodix`  
> **Language**: Go  

---

## 1. Overall Architecture

Melodix is a **Discord music bot** written in Go. It is structured as a **hexagonal (ports-and-adapters) monolith** with four layers:

```
cmd/            ── Entry points (Discord bot, CLI)
internal/       ── Application core (commands, Discord wiring, storage, config)
pkg/            ── Reusable libraries (music playback, streaming, opus)
docs/           ── Documentation
docker/         ── Containerization
```

The main binary (`cmd/discord/main.go`) initializes configuration, storage, and the Discord session, then blocks on signal handling with an automatic session-reconnect loop.

---

## 2. Dependency Graph

```
cmd/discord/main.go
 │
 ├── internal/config          (Env-based configuration)
 ├── internal/applog          (Zerolog setup, file rotation)
 ├── internal/storage         (JSON file persistence for history)
 ├── internal/discord         (Bot core, session lifecycle, handlers)
 │   ├── cmdadapter           (Σ → command.Adapter bridge)
 │   ├── cmdlogger            (Command invocation logging)
 │   ├── cmdsync              (Slash command registration)
 │   ├── execguard            (Concurrency limiter)
 │   ├── perm                 (Voice permission checks)
 │   ├── reply                (Embed helpers)
 │   ├── voice/service.go     (Voice lifecycle, player management)
 │   │   └── voice/sink       (Discord voice connection + Opus streaming)
 │   └── watchdog             (WebSocket silence + health tracker)
 │
 ├── internal/command         (Command implementations)
 │   ├── core/{about,help,commands,maintenance}
 │   ├── music/{play,stop,next,history,common}
 │   └── settings/
 │
 ├── pkg/music/player         (Queue-based playback engine)
 │   ├── stream               (RecoveryStream — retryable media opener)
 │   ├── parsers              (Media parser registry: ytnative, scnative, ytdlp, kkdai, ffmpeg)
 │   ├── sources              (Source detection: YouTube, SoundCloud, Radio)
 │   ├── resolve              (URL resolution: playlist → tracks)
 │   ├── sink                 (AudioSink interface + speaker provider)
 │   ├── opus                 (Opus frame demux/encode)
 │   └── soundcloudapi        (SoundCloud API client)
 │
 └── pkg/discordgo-fork-dev   (Forked discordgo with voice fixes)
```

---

## 3. Folder Responsibilities

| Folder | Responsibility |
|--------|---------------|
| `cmd/discord/` | Discord bot entry point: config → storage → bot → signal loop |
| `cmd/cli/` | CLI player entry point (non-Discord playback) |
| `internal/config/` | Env-file + environment variable config (godotenv + caarlos0/env) |
| `internal/applog/` | Zerolog setup with optional file rotation (lumberjack) |
| `internal/storage/` | JSON file-based persistence for music playback history |
| `internal/domain/` | Domain types (minimal — playback history) |
| `internal/discord/` | Bot struct, session lifecycle, discordgo handler wiring |
| `internal/discord/cmdadapter/` | Bridge between `github.com/keshon/command` registry and discordgo interactions |
| `internal/discord/cmdlogger/` | Logs command invocations to persistent storage |
| `internal/discord/cmdsync/` | Syncs slash commands to guilds (hash-based diff) |
| `internal/discord/execguard/` | Command concurrency limiter (max N concurrent, timeout) |
| `internal/discord/perm/` | Discord voice channel permission checks |
| `internal/discord/reply/` | Embed builders for music status, error, "now playing" |
| `internal/discord/voice/` | Voice service: per-guild player + sink provider management |
| `internal/discord/voice/sink/` | Discord voice connection management + Opus streaming |
| `internal/discord/watchdog/` | WebSocket silence detection + heartbeat tracker |
| `internal/command/` | All slash command implementations |
| `internal/command/core/` | Non-music commands (about, help, commands list, maintenance) |
| `internal/command/music/` | Music commands (play, stop, next, history) |
| `internal/command/music/common/` | Shared music logic (input parsing, error descriptions) |
| `internal/command/settings/` | Guild settings command |
| `internal/middleware/` | Command middleware (group access, guild-only, permissions, logging) |
| `internal/playbackerr/` | Playback error message formatting |
| `internal/readme/` | Auto-generated README from command registry |
| `pkg/music/player/` | Core playback engine: queue, startTrack, runPlayback |
| `pkg/music/stream/` | RecoveryStream: retryable media stream opening with transient error handling |
| `pkg/music/parsers/` | Media parser implementations (ffmpeg, kkdai, ytdlp, ytnative, scnative) |
| `pkg/music/sources/` | Source detection (YouTube, SoundCloud, Radio) + TrackInfo types |
| `pkg/music/resolve/` | URL resolver: playlist detection, track listing from URLs |
| `pkg/music/sink/` | AudioSink interface + local speaker provider |
| `pkg/music/opus/` | Opus frame read/decode |
| `pkg/music/soundcloudapi/` | SoundCloud API client |
| `docs/` | Architecture, conventions, running docs |
| `docker/` | Dockerfile, docker-compose, build scripts |
| `assets/` | Images (avatar, banners) |

---

## 4. Discord Lifecycle

```
main()
  ├── signal.NotifyContext (SIGINT/SIGTERM)
  ├── config.NewConfig()
  ├── applog.Setup()
  ├── storage.NewStorage()
  ├── discord.NewBot()
  ├── registerCommands()  // Wire 8 commands
  ├── go bot.RunSession() // In reconnect loop
  │    │
  │    └── RunSession(ctx)
  │         ├── discordgo.New("Bot " + token)
  │         ├── cmdlogger.NewLogger()
  │         ├── cmdsync.NewSyncer()
  │         ├── attachDiscordgoLogger()
  │         ├── execguard.New(timeout, parallelism)
  │         ├── watchdog.NewTracker()
  │         ├── wireSessionHandlers()
  │         │    ├── onAnyEvent → tracker.MarkWSNow()
  │         │    ├── onReady    → tracker.MarkReadyNow() + onReady()
  │         │    ├── onGuildCreate
  │         │    ├── onMessageCreate
  │         │    ├── onMessageReactionAdd
  │         │    └── onInteractionCreate
  │         ├── dg.Open()  // Connect to Discord gateway
  │         ├── startSessionHealthWatchers()
  │         ├── select { <-ctx.Done, <-disconnected }
  │         └── return (triggers reconnect in main loop)
  │
  ├── <-rootCtx.Done()
  ├── wg.Wait()
  ├── store.Close()
  └── log.Info("bot_exit")
```

**Reconnect behavior**:  
`RunSession` blocks until either `ctx` is cancelled (clean shutdown) or the session is deemed unhealthy. Main loop restarts with a delay — 0–200ms random for unhealthy errors, 5s otherwise.

---

## 5. Slash Command Architecture

### Command Interface Hierarchy

```
command.Command (github.com/keshon/command)
  └── cmdadapter.Adapter
       └── cmdadapter.Handler  (interface)
            ├── Name(), Description(), Group(), Category()
            ├── UserPermissions() []int64
            ├── SlashDefinition() *discordgo.ApplicationCommand  [optional]
            ├── ContextDefinition()                              [optional]
            ├── ReactionDefinition()                             [optional]
            ├── Component(ctx)                                   [optional]
            └── Run(ctx interface{}) error                       [required]
```

### Dispatch Flow

```
discordgo interaction event
  → onInteractionCreate()
    → onApplicationCommand()
      → command.DefaultRegistry.Get(name)
      → Build SlashInteractionContext
      → runGuardedInteraction()
        → runWithCommandContext()
          → guard.Acquire()        // Semaphore
          → command.Run(ctx, inv)  // With timeout context
            → Adapter.Run()
              → cmd.Run(inv.Data)  // e.g. play.Play.Run()
          → guard.Release()
```

### Middleware Chain

Commands are registered with middleware layers:
1. `WithGroupAccessCheck()` — restrict by group
2. `WithGuildOnly()` — reject DMs
3. `WithUserPermissionCheck()` — check user permissions
4. `WithCommandLogger(log)` — log invocations

### Registered Commands

| Name | Handler File | Module |
|------|-------------|--------|
| `play` | `internal/command/music/play/play.go` | Music |
| `stop` | `internal/command/music/stop/stop.go` | Music |
| `next` | `internal/command/music/next/next.go` | Music |
| `history` | `internal/command/music/history/history.go` | Music |
| `about` | `internal/command/core/about/` | Core |
| `help` | `internal/command/core/help/` | Core |
| `commands` | `internal/command/core/commands/` | Core |
| `settings` | `internal/command/settings/settings.go` | Settings |
| `maintenance` | `internal/command/core/maintenance/` | Core |

### Slash Definitions

Commands that implement `SlashProvider` return a `SlashDefinition()` with options:
- **play**: `input` (required string), optional `source` (enum), optional `parser` (enum)
- **stop**: no options
- **next**: no options
- **history**: no options
- **others**: per-command definitions

---

## 6. Voice Subsystem

```
Service (internal/discord/voice/service.go)
  │
  ├── Per-guild state:
  │   ├── players       map[string]*player.Player
  │   ├── sinkProviders map[string]*DiscordSinkProvider
  │   ├── guildMusicStatus        map[string]{ChannelID, MessageID}
  │   └── guildMusicNotifyChannel map[string]string
  │
  ├── GetOrCreatePlayer(guildID)
  │   ├── Check/create sink.DiscordSinkProvider
  │   ├── player.NewWithOptions(provider, resolver, opts)
  │   ├── p.SetGuildID(guildID)
  │   ├── p.SetRecorder(playbackRecorder)  // persist to storage
  │   ├── go watchPlayerStatus(guildID, p) // async status consumer
  │   └── return p
  │
  ├── ResolveTracks(input, source, parser) → resolve.Resolver
  ├── UpdatePlaybackStatus(session, interaction, guildID, embed)
  ├── StopAllPlayers()
  ├── InvalidateAllSinks()
  └── notifyPlaybackFailed(guildID, track, err)
       └── deliverPlaybackFailureEmbed(session, guildID, embed)
            ├── Edit stored status message, OR
            └── Send new message to notify channel
```

### Sink Provider (Discord Voice Connection)

```
DiscordSinkProvider (internal/discord/voice/sink/provider_discord.go)
  ├── Sink(target)         → join VC + return DiscordSink
  ├── ReleaseSink(target)  → disconnect from VC
  └── InvalidateSink()     → disconnect without target match (for recovery)

DiscordSink (internal/discord/voice/sink/sink_discord.go)
  └── Stream(r opus.Reader, stop <-chan struct{})
       └── streamToDiscord()
            ├── Warm-up: drain 10 packets
            ├── Skip silence: up to 150 near-silent packets
            ├── Send first audible packet
            └── Loop: read → sendOpus(vc.OpusSend) until EOF or stop
                 └── Timeout 3s → ErrVoiceTransport
                 └── Panic from closed ch → ErrVoiceTransport
```

---

## 7. Player Lifecycle

```
Player.New(sinkProvider, resolver)
  │
  ├── Enqueue(input, source, parser)
  │   └── resolver.Resolve() → TrackInfo[] → parsers.Track[] → append queue
  │
  ├── EnqueueTrackInfo(trackInfo)  // Pre-resolved (from history)
  │
  ├── PlayNext(target)
  │   ├── Stop(false) if playing
  │   ├── Dequeue first track
  │   ├── startTrack(track, resumed=false)
  │   │   ├── New RecoveryStream
  │   │   ├── rs.Open(0)  // Open media (retries on transient errors)
  │   │   ├── Set playing=true, currTrack=track
  │   │   ├── emitStatus(StatusPlaying)
  │   │   └── go runPlayback()
  │   │        ├── sink.Sink(target)    // Join voice channel
  │   │        ├── sink.Stream(rs, stopCh)  // Send Opus packets
  │   │        ├── On ErrVoiceTransport → transport recovery (hard/soft)
  │   │        │   ├── Soft: reopen media stream (up to N attempts)
  │   │        │   ├── Hard: invalidate sink, rejoin VC, reopen stream
  │   │        │   └── Max 3 voice transport attempts
  │   │        └── On success/EOF → close doneCh
  │   │
  │   ├── Recorder.Record(guildID, time, track)  // Persist history
  │   └── Return nil
  │
  ├── goroutine after runPlayback:
  │   └── PlayNext(target)  // Auto-advance
  │       ├── Queue empty → Stop(true) → ReleaseSink + leave VC
  │       └── Queue non-empty → startTrack(next)
  │
  ├── Stop(disconnect)
  │   ├── Close stopPlayback channel
  │   ├── Wait for playbackDone (10s timeout)
  │   ├── Set playing=false, currTrack=nil
  │   ├── If disconnect: clear queue, ReleaseSink, leave VC
  │   ├── Reset stopPlayback/playbackDone channels
  │   └── emitStatus(StatusStopped)
  │
  ├── Pause() → ErrPauseNotSupported
  ├── Resume() → ErrResumeNotSupported
  └── CurrentTrack(), Queue(), IsPlaying()
```

### Transport Recovery Flow

```
runPlayback → sink.Stream() returns ErrVoiceTransport
  ├── mode=hard: sinkProvider.InvalidateSink()
  │              rs.ReopenAfterTransportFailure()
  │              retry (sink.Sink → rejoin VC → stream)
  │
  └── mode=soft (up to softAttempts):
       rs.ReopenAfterTransportFailure()
       if fails: fall back to hard
       if retries exhausted: return error
```

---

## 8. Queue Management

- **Data structure**: `[]parsers.Track` slice (FIFO) inside `Player.queue`
- **Enqueue**:  
  - `Enqueue(input, source, parser)` — resolves input via `Resolver`, appends to queue  
  - `EnqueueTrackInfo(trackInfo)` — skips resolution (used for history replay)  
- **Dequeue**:  
  - `PlayNext(target)` — pops `queue[0]`, starts playback  
  - If start fails and queue is non-empty: loop and try next track  
  - If queue empty after fail: return `ErrTrackStartFailed`  
- **Stop with disconnect**: `queue = nil` (full clear)  
- **Tracking**: `Queue()` returns a `slices.Clone()` copy (safe snapshot)  
- **Limits**:  
  - `common.ParsePlayInput()` enforces max items per command (~20 for history IDs, 1-3 for URLs/queries)  
  - No hard queue size limit beyond memory  

---

## 9. Guild State Management

State is distributed across three maps in `voice.Service`:

| Map | Key | Value | Purpose |
|-----|-----|-------|---------|
| `players` | guildID | `*player.Player` | Per-guild playback engine |
| `sinkProviders` | guildID | `*DiscordSinkProvider` | Per-guild voice connection provider |
| `guildMusicStatus` | guildID | `{ChannelID, MessageID}` | "Now Playing" message for editing |
| `guildMusicNotifyChannel` | guildID | `channelID` | Fallback channel for failure messages |

Additionally:
- `Bot.slashCmds` — guild→slash command mapping (for sync)
- `Bot.cfg.DiscordGuildBlacklist` — blacklisted guild IDs
- `Bot.storage` — shared persistence layer (used by all guilds)

**Lifetime**:  
- Created on first `GetOrCreatePlayer(guildID)` call (lazy)  
- Player is stopped and sink released on `Stop(true)` (queue empty)  
- All players stopped on `StopAllPlayers()` (shutdown)  
- Sinks invalidated on `InvalidateAllSinks()` (session recovery)

---

## 10. Session Management

### Session Creation and Teardown

- **Create**: `RunSession(ctx)` creates a new `discordgo.Session`, wires handlers, opens gateway connection
- **Teardown**: On `ctx.Done()` or websocket disconnect → `dg.Close()`, cancel session context, disable exec guard
- **Reconnect**: Main loop in `cmd/discord/main.go` detects `RunSession` return and restarts

### Dependency on Session Object

Commands obtain the session pointer at dispatch time from `SlashInteractionContext.Session`. The voice subsystem uses a `SessionGetter` closure:

```go
type SessionGetter func() *discordgo.Session
```

This indirection ensures sink providers and status updaters always use the **current session**, even after a reconnection creates a new `discordgo.Session` object.

### Session Health Detection

Two concurrent health watchers run per session:

1. **WebSocket Silence Detector** (`watchdog.NewWSSilence`):
   - Monitors time since last gateway event
   - Configurable timeout (`WS_SILENCE_TIMEOUT`, default 2m)
   - 15s settle delay at startup, 10s tick interval

2. **API Probe** (in `startSessionHealthWatchers`):
   - Starts after 15s delay
   - Every 30s: calls `dg.User("@me")`
   - 3 consecutive failures → unhealthy

### Unhealthy Response

Controlled by `DiscordUnhealthyMode` (`restart-session` | `restart-voice` | `ignore`):
- `restart-voice`: calls `InvalidateAllSinks()` (disconnect VC)
- `restart-session` (default): closes `disconnected` channel → `RunSession` returns → main loop reconnects
- `grace` setting: allows N unhealthy events within a window before full restart (still invalidates sinks)

---

## 11. Background Goroutines

| Goroutine | Created By | Purpose | Lifespan |
|-----------|-----------|---------|----------|
| **Session reconnect loop** | main() | Restarts `RunSession` on failure | Bot lifetime |
| **runPlayback** | startTrack() | Streams Opus to Discord per track | Per-track |
| **Auto-advance** | startTrack() (goroutine after runPlayback) | Calls PlayNext when track ends | Per-track |
| **watchPlayerStatus** | GetOrCreatePlayer() | Consumes `PlayerStatus` channel for async UI updates | Per-player (guild lifetime) |
| **WS silence monitor** | startSessionHealthWatchers() | Goroutine in watchdog | Per-session |
| **API probe** | startSessionHealthWatchers() | Periodic `User("@me")` check | Per-session |
| **Storage background operations** | storage.NewStorage() | Periodic cleanup, flush | Per-storage |

**Note**: `watchPlayerStatus` is the single long-lived consumer of each player's `PlayerStatus` channel. This is important — commands emit status synchronously via their interaction response, and the watcher handles only async transitions (auto-advance, queue-end).

---

## 12. Configuration Loading

```
NewConfig()
  ├── godotenv.Load()         // Load .env file (optional, warns on missing)
  └── env.Parse(&cfg)         // Overlay with env vars (caarlos0/env)
```

### Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCORD_TOKEN` | — | Bot token (required for Discord) |
| `DISCORD_GUILD_BLACKLIST` | — | Comma-separated guild IDs to auto-leave |
| `STORAGE_PATH` | `./data/datastore.json` | JSON history file |
| `DEVELOPER_ID` | — | Discord user ID for owner commands |
| `INIT_SLASH_COMMANDS` | `false` | Auto-register slash cmds on ready |
| `VOICE_READY_DELAY_MS` | `500` | Delay after VC join before sending Opus |
| `COMMAND_TIMEOUT` | `30s` | Max time for command execution |
| `COMMAND_PARALLELISM` | `16` | Max concurrent commands |
| `WS_SILENCE_TIMEOUT` | `2m` | Gateway silence timeout |
| `DISCORD_UNHEALTHY_MODE` | `restart-session` | Session health action |
| `DISCORD_UNHEALTHY_GRACE` | `0` | Grace count before session restart |
| `DISCORD_UNHEALTHY_WINDOW` | `1m` | Grace count window |
| `PLAYER_TRANSPORT_RECOVERY_MODE` | `hard` | Voice transport recovery strategy |
| `PLAYER_TRANSPORT_SOFT_ATTEMPTS` | `1` | Soft recovery retries |
| `LOG_LEVEL` | `info` | Zerolog level |
| `LOG_FILE` | — | Log file path (stderr if empty) |
| `LOG_MAX_SIZE_MB` | `10` | Log rotation max MB |
| `LOG_MAX_BACKUPS` | `3` | Log rotation max backups |
| `LOG_MAX_AGE_DAYS` | `0` | Log rotation max days |
| `LOG_COMPRESS` | `false` | Log rotation compress |

---

## 13. Error Handling

### Strategy

Errors are handled at four levels:

1. **Command handler** (e.g., `play.Play.Run`):  
   - Returns user-facing embeds via `reply.RespondEmbedEphemeral` / `reply.FollowupEmbedEphemeral`  
   - Logs unexpected errors via `slashCtx.AppLog.Warn()`  
   - Returns `nil` to command framework after sending user message

2. **Command guard** (`runWithCommandContext` / `runGuardedInteraction`):  
   - Busy: sends ephemeral "Bot is busy" embed  
   - Timeout: sends ephemeral "Timed out" embed  
   - Error: sends ephemeral "Error running command" + logs at Error level

3. **Player** (`Player.runPlayback`):  
   - `ErrSinkUnavailable`: returns early without auto-advance  
   - `ErrVoiceTransport`: triggers transport recovery (hard/soft)  
   - `ErrPlaybackStopped`: clean user stop  
   - `onPlaybackFailed` callback: delivers failure embed to guild channel

4. **Session** (`RunSession`):  
   - Setup errors returned to main loop for retry  
   - Health watchers close `disconnected` channel → session restart

### Error Types

| Package | Error | When |
|---------|-------|------|
| `player` | `ErrNoTrackPlaying` | No current track |
| `player` | `ErrNoTracksInQueue` | Queue empty on PlayNext |
| `player` | `ErrTrackStartFailed` | All parsers failed + queue empty |
| `player` | `ErrNoParsersForTrack` | Track has zero available parsers |
| `player` | `ErrSinkUnavailable` | Voice join failed |
| `stream` | `ErrPlaybackStopped` | Stop signal received |
| `stream` | `ErrVoiceTransport` | Opus send timeout/panic |
| `stream` | `ErrTransientHTTP` | Retryable HTTP error |
| `storage` | `ErrMusicPlaybackNotFound` | History ID not found |
| `common` | `ErrPlayInputTooManyItems` | Too many items in one command |
| `discord` | `ErrSessionUnhealthy` | Session deemed unhealthy |

---

## 14. Extension Points

### Adding a New Slash Command

1. Create package in `internal/command/<group>/<name>/`
2. Implement `cmdadapter.Handler` interface (Name, Description, Group, Category, UserPermissions, Run, optional SlashDefinition)
3. Register in `cmd/discord/main.go` → `registerCommands()` via `cmdadapter.Register()`

### Adding a New Media Source

1. Create package in `pkg/music/sources/<name>/`
2. Implement source resolution logic
3. Add parser in `pkg/music/parsers/<name>/`
4. Register in source detection (`pkg/music/sources/sources.go` / `pkg/music/sources/url.go`)

### Config Extensions

Add fields to `internal/config/config.go` with env tags and defaults, used anywhere in the application.

### Command Middleware

Add to `internal/middleware/` and include in `defaultMiddleware()` in `cmd/discord/main.go`.

---

## 15. Risks and Technical Debt

### Medium Risk

1. **No queue size limit**: `Player.queue` is an unbounded slice. A malicious user or bug could enqueue thousands of tracks, consuming memory until OOM.

2. **JSON file storage**: `storage.NewStorage()` uses a single JSON file (`./data/datastore.json`). Concurrent writes from multiple guilds rely on read/write locks. At scale, this becomes a contention bottleneck. No backup/corruption recovery mechanism.

3. **Non-atomic player stop**: `Stop(disconnect)` has a race window between checking `IsPlaying()` and closing `stopPlayback` — `playbackDone` may be stale. The 10-second timeout on wait is a safety valve but could mask bugs.

4. **Voice join timeout is fixed**: `voiceJoinTimeout = 15s` is hardcoded. Network conditions may require adjustment, but there's no config knob.

5. **`watchPlayerStatus` goroutine leak**: One goroutine per guild runs for the player's lifetime. On a large bot (thousands of guilds), this could be significant. No limit on number of guilds joined.

### Low Risk

6. **Panic in `sendOpus`**: Recovered from closed `OpusSend` channel, but `recover()` catches all panics, potentially masking unrelated issues.

7. **`guildMusicStatus` map growth**: Maps grow unboundedly per guild. No cleanup on guild leave (the bot leaves, but entries persist in memory).

8. **No soft delete for storage**: History entries accumulate indefinitely. No TTL or max-entries cap.

9. **`Player.Enqueue()` vs `EnqueueTrackInfo()`**: Two paths with nearly identical logic. The command handler bypasses `Enqueue()` (which calls `Resolve()` again) by calling `EnqueueTrackInfo()` directly. This is intentional but fragile — if `Enqueue` adds logic later, `EnqueueTrackInfo` must be updated too.

10. **`DiscordUnhealthyGrace` off by default**: Grace is 0, so the first unhealthy event triggers a full session restart. During transient Discord API hiccups this may cause unnecessary reconnections.

### Design Observations

11. **Command interface uses `interface{}`**: `Run(ctx interface{})` takes a raw interface that must be type-asserted to `*SlashInteractionContext`. No compile-time safety.

12. **Pause/Resume stubs**: `Pause()` and `Resume()` return `ErrPauseNotSupported`. The entire sink streaming model would need restructuring to support pause.

13. **RecoveryStream concurrency**: `rs.Open(0)` and `rs.ReopenAfterTransportFailure()` are called from different goroutines without documented synchronization.

---

## 16. Architecture Diagrams

### 16.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        cmd/discord/main.go                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │ config   │  │ storage  │  │ applog   │  │ signal (SIGINT)    │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────────────────────┘ │
│       │              │             │                                │
│       ▼              ▼             ▼                                │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                     discord.Bot                              │   │
│  │  ┌──────────────────────────────────────────────────────┐   │   │
│  │  │               voice.Service                          │   │   │
│  │  │  ┌──────────────┐  ┌──────────────┐  ┌────────────┐ │   │   │
│  │  │  │ Player(guild)│  │ Player(guild)│  │ ...        │ │   │   │
│  │  │  │  ┌──────────┐│  │              │  │            │ │   │   │
│  │  │  │  │ Queue    ││  │              │  │            │ │   │   │
│  │  │  │  │ Resolver ││  │              │  │            │ │   │   │
│  │  │  │  │ SinkProv ││  │              │  │            │ │   │   │
│  │  │  │  │ StatusCh ││  │              │  │            │ │   │   │
│  │  │  │  └──────────┘│  │              │  │            │ │   │   │
│  │  │  └──────────────┘  └──────────────┘  └────────────┘ │   │   │
│  │  └──────────────────────────────────────────────────────┘   │   │
│  │                                                              │   │
│  │  ┌──────────────────────────────────────────────┐            │   │
│  │  │         command.DefaultRegistry              │            │   │
│  │  │  /play  /stop  /next  /history  /about       │            │   │
│  │  │  /help  /commands  /settings  /maintenance   │            │   │
│  │  └──────────────────────────────────────────────┘            │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 16.2 Voice Subsystem

```
Interaction (slash command)
  │
  ▼
VoiceAPI.GetOrCreatePlayer(guildID)
  │
  ├──► voice.Service
  │     ├── Check player map │ create new Player
  │     ├── Create DiscordSinkProvider
  │     │     └── SessionGetter (closure for current Session)
  │     ├── player.NewWithOptions(provider, resolver, opts)
  │     ├── p.SetGuildID(guildID)
  │     ├── p.SetRecorder(playbackRecorder → storage)
  │     └── go watchPlayerStatus(guildID, p)
  │
  └──► Player
        ├── EnqueueTrackInfo(track) → append to queue
        ├── PlayNext(target)
        │     ├── DiscordSinkProvider.Sink(target)
        │     │     ├── dg.ChannelVoiceJoin(guildID, target)
        │     │     ├── sleep(voiceReadyDelay)
        │     │     └── return DiscordSink{vc}
        │     ├── startTrack(track)
        │     │     ├── RecoveryStream.Open()
        │     │     ├── go runPlayback(track, rs, stopCh, doneCh)
        │     │     │     ├── DiscordSink.Stream(rs, stopCh)
        │     │     │     │     └── sendOpus(vc.OpusSend)
        │     │     │     ├── On ErrVoiceTransport → recover()
        │     │     │     └── On EOF → close(doneCh)
        │     │     └── go auto-advance:
        │     │           PlayNext(target) → next track or Stop(true)
        │     └── Record(guildID, time, track) → storage
        │
        └── watchPlayerStatus
              ├── StatusPlaying → UpdatePlaybackStatus(NowPlayingEmbed)
              ├── StatusStopped → UpdatePlaybackStatus(PlaybackFinishedEmbed) if queue empty
              └── StatusError → deliverPlaybackFailedEmbed to guild
```


## 17. File-by-File Source Analysis

This section catalogs every meaningful source file by role, exports, and key design decisions.

### cmd/

| File | Role | Key |
|------|------|-----|
| `cmd/discord/main.go` | Bot entry point | Signal loop, session reconnect, command registration |
| `cmd/cli/main.go` | CLI player entry point | Terminal-based playback (no Discord) |

### internal/config/

| File | Role |
|------|------|
| `config.go` | Struct + env parsing via caarlos0/env + godotenv |
| `config_test.go` | Coverage for default/override behavior |

### internal/applog/

| File | Role |
|------|------|
| `log.go` | Zerolog writer setup with optional lumberjack rotation |
| `log_test.go` | Level parsing, file rotation edge cases |

### internal/storage/

| File | Role | Key |
|------|------|-----|
| `storage.go` | JSON file read/write with RWMutex | Single file persistence; `datastore.json` |
| `migration.go` | Schema migration for history format changes | Version-keyed migration |
| `history.go` | CRUD for `MusicPlaybackHistory` | AddPlayback, GetPlayback, GetPlaybackList |
| `storage_test.go` | Concurrent read/write test | Verifies RWMutex correctness |
| `migration_test.go` | Migration version tests | |
| `history_test.go` | History CRUD tests | |

### internal/domain/

| File | Role |
|------|------|
| `playback_history.go` | `MusicPlaybackHistory` + `MusicPlaybackEntry` structs |

### internal/discord/

| File | Role | Key |
|------|------|------|
| `bot.go` | Bot struct, session creation, handler wiring | `NewBot()`, `RunSession()` |
| `handlers.go` | discordgo event handlers: onReady, onGuildCreate, etc. | Session health marks, slash command sync |
| `handlers_interactions.go` | Interaction dispatch: commands, components, autocomplete | `onInteractionCreate` |
| `opts.go` | BotOptions struct (func-option pattern) | |

#### internal/discord/cmdadapter/

| File | Role |
|------|------|
| `interfaces.go` | Handler, Responder, Logger, SlashProvider, etc. interfaces |
| `adapter.go` | Adapter bridging command registry to discordgo interactions |
| `context.go` | SlashInteractionContext: session, interaction, guildID, etc. |

#### internal/discord/cmdlogger/

| File | Role |
|------|------|
| `cmdlogger.go` | Logs command invocations to storage with timestamp |

#### internal/discord/cmdsync/

| File | Role | Key |
|------|------|------|
| `syncer.go` | Hash-based slash command diff sync | Only updates changed commands |
| `syncer_test.go` | Sync logic tests | |

#### internal/discord/execguard/

| File | Role |
|------|------|
| `execguard.go` | Command concurrency limiter (semaphore + timeout) |

#### internal/discord/perm/

| File | Role |
|------|------|
| `perm.go` | Discord voice channel permission checks |

#### internal/discord/reply/

| File | Role |
|------|------|
| `reply.go` | DefaultResponder, embed builders, interaction reply helpers |
| `reply_test.go` | Embed construction tests |

#### internal/discord/voice/

| File | Role | Key |
|------|------|------|
| `service.go` | Voice service: per-guild player management | `GetOrCreatePlayer`, `ResolveTracks`, `StopAllPlayers` |
| `service_notify.go` | Error delivery: fallback channel message | `deliverPlaybackFailureEmbed` |

#### internal/discord/voice/sink/

| File | Role |
|------|------|
| `provider_discord.go` | DiscordSinkProvider: join/leave VC |
| `sink_discord.go` | DiscordSink: Opus streaming to vc.OpusSend |
| `provider_discord_test.go` | Join timeout tests |

#### internal/discord/watchdog/

| File | Role | Key |
|------|------|------|
| `tracker.go` | WebSocket silence + heartbeat tracker | MarkWSNow, MarkReadyNow, IsHealthy |
| `ws_silence.go` | WSSilence: goroutine monitoring WS gap | Configurable timeout |
| `tracker_test.go` | Health state tests | |
| `ws_silence_test.go` | Timeout detection tests | |

### internal/command/

| File | Role |
|------|------|
| `commands.go` | Central command registry wiring + middleware |

#### internal/command/core/about/

| File | Role |
|------|------|
| `about.go` | `/about`: bot info embed |

#### internal/command/core/commands/

| File | Role |
|------|------|
| `commands.go` | `/commands`: interactive command list |

#### internal/command/core/help/

| File | Role |
|------|------|
| `help.go` | `/help`: usage guide |

#### internal/command/core/maintenance/

| File | Role |
|------|------|
| `maintenance.go` | `/maintenance`: owner-only, restart/cache/stop |

#### internal/command/music/play/

| File | Role | Key |
|------|------|------|
| `play.go` | `/play input: ...`: resolve + enqueue + PlayNext | Main entry command |
| `play_test.go` | Input parsing edge cases | |

#### internal/command/music/stop/

| File | Role |
|------|------|
| `stop.go` | `/stop`: stop playback + disconnect |

#### internal/command/music/next/

| File | Role |
|------|------|
| `next.go` | `/next`: skip to next track |

#### internal/command/music/history/

| File | Role |
|------|------|
| `history.go` | `/history`: show recent tracks, replay via track ID |
| `history_test.go` | History display formatting tests |

#### internal/command/music/common/

| File | Role |
|------|------|
| `play_input.go` | ParsePlayInput: URL/query parsing with max-item limits |
| `play_input_test.go` | Complex input edge case tests |
| `embed_text.go` | Embed text formatting for embeds |
| `embed_text_test.go` | Embed text formatting tests |
| `history_display.go` | History display pagination/formatting |
| `history_display_test.go` | Pagination edge case tests |

#### internal/command/settings/

| File | Role |
|------|------|
| `settings.go` | `/settings`: guild configuration |

### internal/middleware/

| File | Role |
|------|------|
| `middleware.go` | Command middleware: group access, guild-only, permissions, logging |

### internal/playbackerr/

| File | Role |
|------|------|
| `playbackerr.go` | Playback error description formatting for user-facing embeds |

### internal/readme/

| File | Role |
|------|------|
| `readme.go` | Auto-generated README from command registry via cmdadapter.Handler metadata |

### pkg/music/player/

| File | Role | Key |
|------|------|------|
| `player.go` | Core Player struct + Options + NewWithOptions | Queue, PlayNext, Stop, startTrack |
| `player_playback.go` | runPlayback: main streaming loop + transport recovery | Hard/soft voice recovery |
| `player_status.go` | Status watcher channel, async UI updates | watchPlayerStatus goroutine |
| `player_queue.go` | Enqueue, EnqueueTrackInfo, Dequeue, Queue snapshot | FIFO slice, play-all logic |
| `player_error.go` | Error definition functions for transport recovery | |
| `player_test.go` | Enqueue/PlayNext/Stop lifecycle tests | |

### pkg/music/stream/

| File | Role |
|------|------|
| `stream.go` | RecoveryStream: retryable media reader with transient error handling |
| `stream_test.go` | Retry logic tests |

### pkg/music/parsers/

| File | Role |
|------|------|
| `parsers.go` | Track struct + Parser interface + registry |
| `parsers_native.go` | ytnative + scnative: native YouTube/SoundCloud extraction |
| `parsers_ytdlp.go` | ytdlp: external yt-dlp binary parser |
| `parsers_kkdai.go` | kkdai: kkdai/youtube parser |
| `parsers_ffmpeg.go` | ffmpeg: generic media parser |
| `parsers_test.go` | Parser selection and fallback tests |

### pkg/music/sources/

| File | Role |
|------|------|
| `sources.go` | TrackInfo + SourceType types, source detection dispatch |
| `url.go` | URL parsing: YouTube, SoundCloud, direct media URL detection |

### pkg/music/resolve/

| File | Role |
|------|------|
| `resolve.go` | Resolver: source detection -> parser resolution -> Track list |
| `resolve_test.go` | Resolve integration tests |

### pkg/music/sink/

| File | Role |
|------|------|
| `sink.go` | AudioSink interface + local speaker sink (for CLI mode) |

### pkg/music/opus/

| File | Role |
|------|------|
| `opus.go` | Opus frame reader / decoder |

### pkg/music/soundcloudapi/

| File | Role |
|------|------|
| `soundcloudapi.go` | SoundCloud API client (search + track resolution) |
| `soundcloudapi_test.go` | API client tests |

### pkg/discordgo-fork-dev/

| File | Role |
|------|------|
| (forked discordgo) | Patched discordgo with voice connection fixes |


## 18. Build and Run

### Build

```bash
go build -o melodix ./cmd/discord
```

### Run (Discord)

```bash
DISCORD_TOKEN="your_token" ./melodix
```

### Docker

```bash
docker compose -f docker/docker-compose.yml up
```

### Scripts

| Script | Purpose |
|--------|---------|
| `build-n-run.sh` | Linux/macOS build + run |
| `build-n-run.bat` | Windows build + run |
| `test.bat` | Windows test runner |
| `docker/build-n-deploy.sh` | Docker build + deploy |

---

*Generated from melodix repository at commit `264c050`.*
*Analysis covers config, storage, Discord bot lifecycle, voice subsystem, player, commands, middleware, and all supporting packages.*