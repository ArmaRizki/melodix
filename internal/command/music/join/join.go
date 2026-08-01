package join

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/melodix/internal/discord"
	"github.com/keshon/melodix/internal/discord/cmdadapter"
	"github.com/keshon/melodix/internal/discord/perm"
	"github.com/keshon/melodix/internal/discord/reply"
)

type Join struct {
	Bot discord.VoiceAPI
}

func (c *Join) Name() string             { return "join" }
func (c *Join) Description() string      { return "Make the bot join your voice channel and stay (24/7 mode)" }
func (c *Join) Group() string            { return "music" }
func (c *Join) Category() string         { return "🎵 Music" }
func (c *Join) UserPermissions() []int64 { return []int64{} }

func (c *Join) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *Join) Run(ctx interface{}) error {
	slashCtx, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := slashCtx.Session
	e := slashCtx.Event

	if err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return fmt.Errorf("failed to defer response: %w", err)
	}

	member := e.Member
	guildID := e.GuildID

	voiceState, err := c.Bot.FindUserVoiceState(guildID, member.User.ID)
	if err != nil {
		reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Title:       "🎵 Voice Error",
			Description: "You must be in a voice channel first.",
		})
		return nil
	}

	permOK, err := perm.CheckBotVoicePermissions(s, voiceState.ChannelID)
	if err != nil || !permOK {
		reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Title:       "🎵 Voice Error",
			Description: "I don't have permission to join or speak in that voice channel.",
		})
		return nil
	}

	player := c.Bot.GetOrCreatePlayer(guildID)
	if player == nil {
		reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Title:       "🎵 Error",
			Description: "Music service is not available.",
		})
		return nil
	}

	// Join the voice channel now (the provider caches the connection) and enable 24/7 mode.
	if err := c.Bot.JoinVoiceChannel(guildID, voiceState.ChannelID); err != nil {
		slashCtx.AppLog.Warn().Str("guild_id", guildID).Err(err).Msg("join_voice_channel_failed")
		reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Title:       "🎵 Voice Error",
			Description: "Failed to join the voice channel.",
		})
		return nil
	}

	c.Bot.SetStayConnected(guildID, voiceState.ChannelID, true)

	c.Bot.SetGuildMusicNotifyChannel(guildID, e.ChannelID)

	if err := reply.FollowupEmbed(s, e, &discordgo.MessageEmbed{
		Description: "✅ Joined voice channel. 24/7 mode enabled — I'll stay even when the queue is empty.",
	}); err != nil {
		slashCtx.AppLog.Warn().Str("command", "join").Err(err).Msg("followup_embed_failed")
	}

	return nil
}