// standalone dir manager (draft)
package main

import (
	"fmt"
	"strings"

	dir "github.com/Pyorot/streams/dir"
	log "github.com/Pyorot/streams/log"
	. "github.com/Pyorot/streams/utils"

	"github.com/bwmarrin/discordgo"
)

var err error                  // placeholder error
var discord *discordgo.Session // Discord client
var serverID string            // Discord ID of the server the role belongs to
var gameName string            // Game title as claimed by Twitch

// runs on program start
func init() {
	Env.Load()
	discord, err = discordgo.New("Bot " + Env.GetOrExit("DISCORD"))
	ExitIfError(err)
	err = discord.Open() // usually awaits Ready event
	ExitIfError(err)
	dir.Init(discord, Env.GetOrExit("DIR_CHANNEL"), true)
	serverID = Env.GetOrExit("SERVER")
	gameName = Env.GetOrExit("GAME_NAME")
	discord.AddHandler(dataHandler)
	log.Insta <- ". | ready\n"
}

func main() {
	fmt.Scanln()
	discord.Close()
	fmt.Println(". | exit")
}

func dataHandler(s *discordgo.Session, m *discordgo.PresenceUpdate) {
	if m.Game != nil &&
		m.Game.Name == "Twitch" &&
		m.Game.Type == discordgo.GameTypeStreaming &&
		m.Game.State == gameName &&
		m.GuildID == serverID {
		k, v := m.Game.URL[strings.LastIndex(m.Game.URL, "/")+1:], m.User.ID
		log.Insta <- fmt.Sprintf("d+| %s", k)
		dir.Add(k, v)
	}
}
