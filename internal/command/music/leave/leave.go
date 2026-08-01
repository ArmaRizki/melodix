package leave

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/melodix/internal/discord"
	"github.com/keshon/melodix/internal/discord/cmdadapter"
	"github.com/keshon/melodix/internal/discord/reply"
)

type Leave struct {
	Bot discord.VoiceAPI
}

func (c *Leave) Name() string             { return "leave" }
func (c *Leave) Description() string      { return "Disable 24/7 mode and leave the voice channel" }
func (c *Leave) Group() string            { return "music" }
func (c *Leave) Category() string         { return "🎵 Music" }
func (c *Leave) UserPermissions() []int64 { return []int64{} }

func (c *Leave) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *Leave) Run(ctx interface{}) error {
	slashCtx, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := slashCtx.Session
	e := slashCtx.Event
	guildID := e.GuildID

	if err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return fmt.Errorf("failed to defer response: %w", err)
	}

	player := c.Bot.GetOrCreatePlayer(guildID)
	if player == nil {
		reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Title:       "🎵 Error",
			Description: "Music service is not available.",
		})
		return nil
	}

	// Disable 24/7 stay-connected mode before stopping.
	c.Bot.SetStayConnected(guildID, "", false)

	// Clear queue, stop playback, and disconnect from voice.
	player.ClearQueue()
	player.Stop(true)
	if err := c.Bot.UpdatePlaybackStatus(s, e, guildID, reply.PlaybackFinishedEmbed()); err != nil {
		slashCtx.AppLog.Warn().Str("guild_id", guildID).Err(err).Msg("guild_status_update_failed")
	}

	if err := reply.FollowupEmbed(s, e, &discordgo.MessageEmbed{
		Description: "👋 Left the voice channel and disabled 24/7 mode.",
	}); err != nil {
		slashCtx.AppLog.Warn().Str("command", "leave").Err(err).Msg("followup_embed_failed")
	}

	return nil
}