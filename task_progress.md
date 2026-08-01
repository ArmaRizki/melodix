# 24/7 Voice Channel Presence — Implementation Progress

## Phase 1: Foundation
- [ ] 1. Add `stayConnected` field + accessors to Player
- [ ] 2. Add `StatusIdle` status constant
- [ ] 3. Add `stayConnectedGuilds` + `reconnectTargets` to Service + `SetStayConnected()`
- [ ] 4. Add `DefaultStayConnected` to config
- [ ] 5. Modify auto-advance in `startTrack()` for stay-connected behavior

## Phase 2: New Commands
- [ ] 6. Create `/join` handler
- [ ] 7. Create `/leave` handler
- [ ] 8. Register both commands in cmd/discord/main.go

## Phase 3: Modify Existing Commands
- [ ] 9. Modify `/stop` for 24/7 awareness

## Phase 4: Session Recovery
- [ ] 10. Add `RejoinStayConnectedTargets` to Service
- [ ] 11. Wire into onReady handler

## Phase 5: Polish
- [ ] 12. Add `IdleEmbed` to reply package
- [ ] 13. Add `SetStayConnected` to VoiceAPI interface
- [ ] 14. Handle guild delete cleanup