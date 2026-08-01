package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/melodix/pkg/music/player"
	"github.com/keshon/melodix/pkg/music/sources"
)

// VoiceAPI is the interface the Discord bot exposes for voice/music commands.
type VoiceAPI interface {
	// GetOrCreatePlayer returns an existing player for the guild or creates a new one.
	GetOrCreatePlayer(guildID string) *player.Player

	// FindUserVoiceState returns the voice channel a user is currently in, or an error if none.
	FindUserVoiceState(guildID, userID string) (*UserVoiceState, error)

	// Resolve resolves input to tracks using the bot's shared resolver.
	ResolveTracks(guildID, input, source, parser string) ([]sources.TrackInfo, error)

	// UpdatePlaybackStatus creates or edits the guild's music status message so updates work beyond 15 min token expiry.
	UpdatePlaybackStatus(s *discordgo.Session, i *discordgo.InteractionCreate, guildID string, embed *discordgo.MessageEmbed) error

	// SetGuildMusicNotifyChannel stores the slash command text channel for async playback failure UI.
	SetGuildMusicNotifyChannel(guildID, channelID string)

	// JoinVoiceChannel connects the bot to a voice channel without starting playback.
	JoinVoiceChannel(guildID, channelID string) error

	// SetStayConnected enables or disables 24/7 stay-connected mode for a guild.
	// When true, the bot remains in voice after the queue empties.
	SetStayConnected(guildID, voiceChannelID string, stay bool)

	// IsStayConnected reports whether the guild is in 24/7 stay-connected mode.
	IsStayConnected(guildID string) bool
}

// UserVoiceState holds minimal voice channel state for a user.
type UserVoiceState struct {
	ChannelID string
	UserID    string
}

// GetOrCreatePlayer returns an existing player for the guild or creates a new one (delegates to voice service).
func (b *Bot) GetOrCreatePlayer(guildID string) *player.Player {
	if b.voice == nil {
		return nil
	}
	return b.voice.GetOrCreatePlayer(guildID)
}

// FindUserVoiceState returns the voice channel a user is currently in, or an error if none.
func (b *Bot) FindUserVoiceState(guildID, userID string) (*UserVoiceState, error) {
	guild, err := b.dg.State.Guild(guildID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving guild: %w", err)
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return &UserVoiceState{ChannelID: vs.ChannelID, UserID: vs.UserID}, nil
		}
	}
	return nil, fmt.Errorf("user not in any voice channel")
}

// ResolveTracks resolves input to tracks using the bot's shared resolver (delegates to voice service).
func (b *Bot) ResolveTracks(guildID, input, source, parser string) ([]sources.TrackInfo, error) {
	if b.voice == nil {
		return nil, fmt.Errorf("voice service not available")
	}
	return b.voice.ResolveTracks(guildID, input, source, parser)
}

// UpdatePlaybackStatus creates or edits the guild's music status message (delegates to voice service).
func (b *Bot) UpdatePlaybackStatus(s *discordgo.Session, i *discordgo.InteractionCreate, guildID string, embed *discordgo.MessageEmbed) error {
	if b.voice == nil {
		return nil
	}
	return b.voice.UpdatePlaybackStatus(s, i, guildID, embed)
}

// SetGuildMusicNotifyChannel records the text channel for public playback-failure fallback (voice service).
func (b *Bot) SetGuildMusicNotifyChannel(guildID, channelID string) {
	if b.voice == nil {
		return
	}
	b.voice.SetGuildMusicNotifyChannel(guildID, channelID)
}

// JoinVoiceChannel connects the bot to a voice channel without starting playback.
func (b *Bot) JoinVoiceChannel(guildID, channelID string) error {
	if b.voice == nil {
		return fmt.Errorf("voice service not available")
	}
	return b.voice.JoinVoiceChannel(guildID, channelID)
}

// SetStayConnected enables or disables 24/7 stay-connected mode for a guild.
func (b *Bot) SetStayConnected(guildID, voiceChannelID string, stay bool) {
	if b.voice == nil {
		return
	}
	b.voice.SetStayConnected(guildID, voiceChannelID, stay)
}

// IsStayConnected reports whether the guild is in 24/7 stay-connected mode.
func (b *Bot) IsStayConnected(guildID string) bool {
	if b.voice == nil {
		return false
	}
	return b.voice.IsStayConnected(guildID)
}
