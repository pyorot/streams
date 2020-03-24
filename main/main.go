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

// main.go: main program loop + Twitch data fetching
// msg.go:  managing a streams channel (posting to Discord)
// role.go: managing a streams role (posting to Discord)

var err error                            // placeholder error
var env = make(map[string]string)        // environment variables
var twitch *helix.Client                 // Twitch client
var discord *discordgo.Session           // Discord client
var twicord = make(map[string]string)    // map: twitch user -> Discord user ID
var getStreamsParams helix.StreamsParams // the const argument for getStreams calls
// twicord is used to find Discord users to assign roles to + maybe to filter the msg channel

type stream helix.Stream

// ternary if macro
func ifThenElse(cond bool, valueIfTrue interface{}, valueIfFalse interface{}) interface{} {
	if cond {
		return valueIfTrue
	}
	return valueIfFalse
}

// fatal error macro, used in initialisations
func exitIfError(err error) {
	if err != nil {
		panic(err)
	}
}

// runs on program start
func init() {
	// load env vars from .env file if present
	err := godotenv.Load()
	if err == nil {
		fmt.Printf(". | Env vars loaded from .env\n")
	} else if os.IsNotExist(err) {
		fmt.Printf(". | Env vars pre-loaded\n")
	} else {
		panic(err)
	}

	// register env vars in env map (assert existence)
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
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{env["GAME_ID"]}, // list of games to query
		First:   100,                      // maximum query results (limit is 100)
	}

	// start worker threads
	go msg()

	// async parallel initialisation (see respective functions)
	task1, task2, task3 := msgInit(), roleInit(), twicordInit() // run async tasks, which return channels,
	_, _, _ = <-task1, <-task2, <-task3                         // awaited by doing blocking reads on them
	fmt.Printf(". | initialised\n")
}

// main function (infinite loop)
func main() {
	for {
		new, err := fetch() // synchronous Twitch http call
		if err == nil {
			fmt.Printf("\n< | %s\n", time.Now().Format("15:04:05"))
			msgCh <- new  // post to msgCh, read by msg(), a permanent worker coroutine thread
			go role(*new) // async call to role(), runs as a task (no return)
			time.Sleep(60 * time.Second)
		}
	}
}

// blocking http request to Twitch getStreams
func fetch() (*map[string]*stream, error) {
	dict := make(map[string]*stream)                 // the return dict (twitch username → stream object)
	res, err := twitch.GetStreams(&getStreamsParams) //
	if err == nil && res.StatusCode != 200 {         // reinterpret HTTP error as actual error
		err = fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if err == nil {
		list := res.Data.Streams // result is in list format
		for i := range list {    // recompile into target dict format
			s := stream(list[i])
			dict[strings.ToLower(list[i].UserName)] = &s
		}
	} else {
		fmt.Printf("\nx | < : %s\n", err)
	}
	return &dict, err
}

// non-blocking http req to read twicord data from a Discord chan
// format is a sequence of posts in the format (where dui = Discord userID, tun = Twitch username):
// "twicord<comment>\n<dui1>\s<tun1>\n<dui2>\s<tun2>\n..."
func twicordInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		history, err := discord.ChannelMessages(env["TWICORD_CHANNEL_ID"], 20, "", "", "") // get last 20 msgs
		exitIfError(err)
		for _, msg := range history {
			if len(msg.Content) >= 8 && msg.Content[:7] == "twicord" { // pick msgs starting with "twicord"
				scanner := bufio.NewScanner(strings.NewReader(msg.Content)) // line-by-line iterator
				scanner.Scan()                                              // skip 1st line ("twicord<comment>\n")
				for scanner.Scan() {
					line := scanner.Text()
					splitIndex := strings.IndexByte(line, ' ')       // line is space-delimited
					twicord[line[splitIndex+1:]] = line[:splitIndex] // dict is rhs → lhs
				}
				exitIfError(scanner.Err())
			}
		}
		fmt.Printf(". | twicord loaded [%d]\n", len(twicord))
		res <- true
	}()
	return res
}
