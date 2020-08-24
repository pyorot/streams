// standalone debug tool to log all PresenceUpdate events
// run in folder with .env file; generates pu.csv

package main

import (
	"fmt"
	"os"
	"time"

	. "github.com/Pyorot/streams/src/utils"
	"github.com/bwmarrin/discordgo"
)

var err error                  // placeholder error
var file *os.File              // target file
var discord *discordgo.Session // Discord client
var gameName, serverID string  // params

func init() {
	// env vars
	Env.Load()
	// target file
	file, err = os.Create("./pu.csv")
	ExitIfError(err)
	// discord
	discord, err = discordgo.New("Bot " + Env.GetOrExit("DISCORD"))
	discord.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildPresences)
	ExitIfError(err)
	gameName, serverID = Env.GetOrExit("GAME_NAME"), Env.GetOrExit("SERVER")
	discord.AddHandler(add)
	discord.AddHandler(ready)
	err = discord.Open()
	ExitIfError(err)
	fmt.Printf(". | connected\n")
}

func main() {
	fmt.Scanln()
	file.Close()
	discord.Close()
}

func add(s *discordgo.Session, m *discordgo.PresenceUpdate) {
	filter := m.GuildID == serverID
	if filter {
		since := 0
		if m.Since != nil {
			since = *m.Since
		}
		write(fmt.Sprintf("e, %s, %d, %s, %s\n",
			time.Now().Format("15:04:05"), // <manual timestamp>
			since,                         // 0 (= timestamp in data)
			m.User.ID,                     // <Discord user>
			m.Status,                      // offline/online/idle/dnd
		))
		if m.Game != nil { // get activities from game field
			printGame('g', m.Game)
		}
		for _, a := range m.Activities { // get activities from activities field
			printGame('a', a)
		}
		write("\n")
	}
}

func printGame(t rune, g *discordgo.Game) {
	write(fmt.Sprintf("%c, %d, %40s, %70s, %60s, %30s, %d, %s, %13v, %v\n",
		t,               // g/a
		g.Type,          // 1 (= Streaming)
		g.Name,          // "Twitch"
		g.State,         // <game>
		g.Details,       // <title>
		g.URL,           // <url>
		g.Instance,      // 0
		g.ApplicationID, // ""
		g.TimeStamps,    // {0, 0}
		g.Assets,        // {"twitch:<handle>", "", "", ""}
	))
}

func write(s string) {
	n, err := file.Write([]byte(s))
	if n != len(s) {
		err = fmt.Errorf("failed write: %d/%d", n, len(s))
	}
	ExitIfError(err)
}

func ready(s *discordgo.Session, m *discordgo.Ready) {
	fmt.Println(". | ready")
}
