package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/nicklaw5/helix"
)

var err error                            // placeholder error
var env = make(map[string]string)        // environment variables
var twitch *helix.Client                 // Twitch client
var discord *discordgo.Session           // Discord client
var twicord = make(map[string]string)    // map: twitch username -> Discord user ID
var getStreamsParams helix.StreamsParams // const argument for getStreams calls

type stream helix.Stream

func exitIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func init() {
	// load env vars from .env file
	if _, exists := os.LookupEnv("TWITCH_KEY"); !exists {
		exitIfError(godotenv.Load())
	}

	// register env vars; assert existence
	for _, key := range []string{
		"TWITCH_KEY", "DISCORD_TOKEN", "GAME_ID",
		"CHANNEL_ID", "ROLE_SERVER_ID", "ROLE_ID",
		"TWICORD_CHANNEL_ID",
	} {
		val, exists := os.LookupEnv(key)
		if exists {
			env[key] = val
		} else {
			panic(fmt.Sprintf("Env var '%s' not found.", key))
		}
	}

	// init clients + constants
	twitch, err = helix.NewClient(&helix.Options{ClientID: env["TWITCH_KEY"]})
	exitIfError(err)
	discord, err = discordgo.New("Bot " + env["DISCORD_TOKEN"])
	exitIfError(err)
	getStreamsParams = helix.StreamsParams{GameIDs: []string{env["GAME_ID"]}, First: 60}

	// start threads
	go msg()
}

func main() {
	// async state init
	task1, task2, task3 := msgInit(), roleInit(), twicordInit()
	_, _, _ = <-task1, <-task2, <-task3
	fmt.Printf(". | initialised\n")

	// loop
	for {
		new, err := fetch()
		if err == nil {
			fmt.Printf("\n< | %s\n", time.Now().Format("15:04:05"))
			msgCh <- new
			go role(*new)
			time.Sleep(60 * time.Second)
		}
	}
}

func fetch() (*map[string]*stream, error) {
	dict := make(map[string]*stream)
	res, err := twitch.GetStreams(&getStreamsParams)
	if err == nil && res.StatusCode != 200 { // reinterpret HTTP error as actual error
		err = fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if err == nil {
		list := res.Data.Streams
		for i := range list {
			s := stream(list[i])
			dict[strings.ToLower(list[i].UserName)] = &s
		}
	} else {
		fmt.Printf("\nx | < : %s\n", err)
	}
	// convert to map: Username -> *Stream
	return &dict, err
}

func twicordInit() chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		history, err := discord.ChannelMessages(env["TWICORD_CHANNEL_ID"], 20, "", "", "")
		exitIfError(err)
		for _, msg := range history {
			if len(msg.Content) >= 8 && msg.Content[:7] == "twicord" {
				scanner := bufio.NewScanner(strings.NewReader(msg.Content))
				scanner.Scan()
				for scanner.Scan() {
					line := scanner.Text()
					splitIndex := strings.IndexByte(line, ' ')
					twicord[line[splitIndex+1:]] = line[:splitIndex]
				}
				exitIfError(scanner.Err())
			}
		}
		fmt.Printf(". | twicord loaded [%d]\n", len(twicord))
		res <- true
	}()
	return res
}
