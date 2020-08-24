package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Pyorot/streams/dir"
	. "github.com/Pyorot/streams/utils"

	"github.com/bwmarrin/discordgo"
	"github.com/nicklaw5/helix"
)

// main.go:   main program init and loop + dir init
// fetch.go:  twitch auth + streams data poll
// msg.go:    managing a streams channel (posting to Discord)
// role.go:   managing a streams role (posting to Discord)
// stream.go: stream struct and conversion/filter methods
// utils.go:  macros for if, errors, env vars

var err error                           // placeholder error
var dirEnabled, twitchEnabled bool      // settings flags: guard some inits and parts of the main loop
var twitch *helix.Client                // Twitch client
var discord *discordgo.Session          // Discord client
var filterTags, filterKeywords []string // Twitch tags and title keywords to filter by
var dirLastLoad time.Time               // last time dir was loaded (0 if dir non-existent)

// runs on program start
func init() {
	// structures to handle async inits (lists to collect tasks to await later)
	awaitDir := make(Await, 0)
	awaitMsgAgents := make([]chan (*msgAgent), 0)
	awaitRole := make(Await, 0)

	// core (sync)
	Env.Load()
	discord, err = discordgo.New("Bot " + Env.GetOrExit("DISCORD"))
	ExitIfError(err)

	// filters + icons (sync, all optional)
	if rawTags := Env.GetOrEmpty("FILTER_TAGS"); rawTags != "" {
		filterTags = strings.Split(rawTags, ",")
		Log.Insta <- fmt.Sprintf(". | filter tags [%d]: %s", len(filterTags), filterTags)
	}
	if rawKeywords := Env.GetOrEmpty("FILTER_KEYWORDS"); rawKeywords != "" {
		filterKeywords = strings.Split(rawKeywords, ",")
		Log.Insta <- fmt.Sprintf(". | filter keywords [%d]: %s", len(filterKeywords), filterKeywords)
	}
	if url := Env.GetOrEmpty("MSG_ICON"); url != "" {
		iconURL[0], iconURL[1], iconURL[2] = url, url, url
		if url2 := Env.GetOrEmpty("MSG_ICON_PASS"); url2 != "" {
			iconURL[1], iconURL[2] = url2, url2
			if url3 := Env.GetOrEmpty("MSG_ICON_KNOWN"); url3 != "" {
				iconURL[2] = url3
			}
		}
	}

	// dir (async)
	dirChannel := Env.GetOrEmpty("DIR_CHANNEL")
	if dirEnabled = dirChannel != ""; dirEnabled {
		awaitDir.Add(dir.Init(discord))
		dirLastLoad = time.Now()
	}

	// msg agents (async) [requires msg icons]
	for _, channel := range strings.Split(Env.GetOrEmpty("MSG_CHANNELS"), ",") {
		if channel == "" {
			continue
		} else if channel[0] == '+' || channel[0] == '*' {
			awaitMsgAgents = append(awaitMsgAgents, newMsgAgent(channel[1:], channel[0] == '+')) // run async task (returns channel)
			twitchEnabled = true
		} else {
			panic(fmt.Sprintf("First char of channel ID %s must be * or +", channel))
		}
	}

	// role (async) [requires dir (if used)]
	if roleID = Env.GetOrEmpty("ROLE"); roleID != "" { // if ROLE is missing, user probs doesn't want a role
		serverID = Env.GetOrExit("SERVER") // if ROLE is there but SERVER missing, user probs forgot the server
		awaitDir.Flush()                   // await dir init, needed for role init to identify users with role already set
		awaitRole.Add(roleInit())          // run async task (returns channel)
		twitchEnabled = true
	}

	// twitch (sync) [requires msg agents and role, to determine if it's needed at all]
	if twitchEnabled {
		twitch, err = helix.NewClient(&helix.Options{
			ClientID:     Env.GetOrExit("TWITCH_ID"),
			ClientSecret: Env.GetOrExit("TWITCH_SEC"),
		})
		ExitIfError(err)
		getStreamsParams = helix.StreamsParams{
			GameIDs: []string{Env.GetOrExit("GAME_ID")}, // list of games to query
			First:   100,                                // maximum query results (limit is 100)
		}
	}

	// await parallel init tasks (by doing blocking reads on their returned channels; see rsp. functions)
	for _, task := range awaitMsgAgents {
		msgAgents = append(msgAgents, <-task) // the task will bear an agent
	}
	awaitDir.Flush()
	awaitRole.Flush()
	Log.Insta <- ". | initialised\n"
}

// main function (infinite loop)
func main() {
	for {
		timeout := 15 * time.Second
		now := time.Now()
		// check for dir reload
		if dirEnabled && now.Sub(dirLastLoad) >= 12*time.Hour {
			dir.Load()
			dirLastLoad = now
		}
		// fetch from Twitch and process
		if twitchEnabled {
			new, err := fetch() // synchronous Twitch http call
			if err == nil {
				Log.Bkgd <- fmt.Sprintf("< | %s", now.Format("15:04:05"))
				// prune blocked names
				for user := range new {
					if dir.IsBlocked(user) {
						delete(new, user)
					}
				}
				// send to msg agents (filter if needed)
				var newFiltered map[string]*stream // declare a map to subset "new" on known/filtered users
				for _, a := range msgAgents {
					if a.filtered { // the agents run msg(), a permanent worker coroutine thread that awaits on these channels
						if newFiltered == nil { // lazily compute newFiltered once
							newFiltered = subsetStreams(new)
						}
						a.inCh <- newFiltered
					} else {
						a.inCh <- new
					}
				}
				// send to role agent
				if roleID != "" {
					go role(new) // async call to role(), runs as a one-off task (no return)
				}
				timeout = 60 * time.Second
			}
		}
		time.Sleep(timeout)
	}
}
