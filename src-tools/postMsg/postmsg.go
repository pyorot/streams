// standalone tool to fetch a stream from Twitch and post it to Discord
// run in folder with .env file

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/Pyorot/streams/src/utils"
	"github.com/nicklaw5/helix"

	"github.com/bwmarrin/discordgo"
)

var err error
var channelID, iconURL string
var discord *discordgo.Session
var twitch *helix.Client
var getStreamsParams helix.StreamsParams
var full bool // full message or stub

func init() {
	// argument validation
	if len(os.Args) != 3 {
		exit()
	}
	if os.Args[1] == "d" {
		full = true
	} else if os.Args[1] != "a" {
		exit()
	}
	// env vars
	Env.Load()
	channelID = strings.Split(Env.GetOrExit("MSG_CHANNELS"), ",")[0][1:]
	iconURL = Env.GetOrExit("MSG_ICON")
	// discord
	discord, err = discordgo.New("Bot " + Env.GetOrExit("DISCORD"))
	ExitIfError(err)
	// twitch
	twitch, err = helix.NewClient(&helix.Options{
		ClientID:     Env.GetOrExit("TWITCH_ID"),
		ClientSecret: Env.GetOrExit("TWITCH_SEC"),
	})
	ExitIfError(err)
	res, err := twitch.GetAppAccessToken(nil)
	ExitIfError(err)
	twitch.SetAppAccessToken(res.Data.AccessToken)
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{os.Args[2]},
		First:   100,
	}
}

func main() {
	res, err := twitch.GetStreams(&getStreamsParams)
	if len(res.Data.Streams) == 0 {
		err = fmt.Errorf("no active streams of this game")
	}
	ExitIfError(err)
	fmt.Println(". | fetched")
	var msg *discordgo.MessageSend
	if full {
		msg = &discordgo.MessageSend{Embed: newMsgFromStream(&res.Data.Streams[0])}
	} else {
		msg = newMsgStubFromStream(&res.Data.Streams[0])
	}
	_, err = discord.ChannelMessageSendComplex(channelID, msg)
	ExitIfError(err)
	fmt.Println(". | posted")
}

// what a notification would look like
func newMsgStubFromStream(r *helix.Stream) *discordgo.MessageSend {
	return &discordgo.MessageSend{Content: fmt.Sprintf("%s: %s", r.UserName, r.Title)}
}

// what a message would look like
func newMsgFromStream(r *helix.Stream) *discordgo.MessageEmbed {
	indexUserStart := strings.LastIndexByte(r.ThumbnailURL, '/') + 11
	indexUserEnd := strings.LastIndexByte(r.ThumbnailURL, '-')
	urlUser := r.ThumbnailURL[indexUserStart:indexUserEnd]
	thumbnail := r.ThumbnailURL[:indexUserEnd+1] + "440x248.jpg"
	length := time.Since(r.StartedAt)
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    r.UserName + " is live",
			URL:     "https://twitch.tv/" + urlUser,
			IconURL: iconURL,
		},
		Description: r.Title,

		Color: 0x00ff00,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: thumbnail,
		},

		Footer: &discordgo.MessageEmbedFooter{
			Text: strings.TrimSuffix(length.Truncate(time.Minute).String(), "0s"),
		},
		Timestamp: time.Now().Format("2006-01-02T15:04:05Z"),
	}
}

func exit() {
	fmt.Print(
		"Usage: Posts a message with the currently most-popular stream of the specified game (by gameID).\n",
		"./postmsg a 69 -- stub message for alerts\n",
		"./postmsg d 69 -- full message for display\n",
	)
	os.Exit(0)
}
