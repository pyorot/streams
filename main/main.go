package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Pyorot/streams/dir"
	log "github.com/Pyorot/streams/log"
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

var err error                  // placeholder error
var twitch *helix.Client       // Twitch client
var discord *discordgo.Session // Discord client
var filterRequired bool        // will generate filtered map of incoming streams
var filterTags []string        // Twitch tags to look for
var filterKeywords []string    // title keywords to look for

// runs on program start
func init() {
	// load env vars from .env file if present (else expect they're already loaded)
	Env.Load()

	// core init
	twitch, err = helix.NewClient(&helix.Options{
		ClientID:     Env.GetOrExit("TWITCH_ID"),
		ClientSecret: Env.GetOrExit("TWITCH_SEC"),
	})
	ExitIfError(err)
	discord, err = discordgo.New("Bot " + Env.GetOrExit("DISCORD"))
	ExitIfError(err)
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{Env.GetOrExit("GAME_ID")}, // list of games to query
		First:   100,                                // maximum query results (limit is 100)
	}

	// filter init
	if rawTags := Env.GetOrEmpty("FILTER_TAGS"); rawTags != "" {
		filterTags = strings.Split(rawTags, ",")
		log.Insta <- fmt.Sprintf(". | filter tags [%d]: %s", len(filterTags), filterTags)
	}
	if rawKeywords := Env.GetOrEmpty("FILTER_KEYWORDS"); rawKeywords != "" {
		filterKeywords = strings.Split(rawKeywords, ",")
		log.Insta <- fmt.Sprintf(". | filter keywords [%d]: %s", len(filterKeywords), filterKeywords)
	}
	if dirChannel := Env.GetOrEmpty("DIR_CHANNEL"); dirChannel != "" {
		dir.Init(discord, dirChannel, false) // do this sync cos role init depends on it
	}

	// msg icons init (2,1,0 = known on dir, not known but passed filter, other rsp.)
	if url := Env.GetOrEmpty("MSG_ICON"); url != "" {
		iconURL[0], iconURL[1], iconURL[2] = url, url, url
		if url2 := Env.GetOrEmpty("MSG_ICON_PASS"); url2 != "" {
			iconURL[1], iconURL[2] = url2, url2
			if url3 := Env.GetOrEmpty("MSG_ICON_KNOWN"); url3 != "" {
				iconURL[2] = url3
			}
		}
	}

	// msg agents init (requires msg icons)
	var awaitMsgAgents = make([]chan (*msgAgent), 0) // a list to collect async tasks, to be awaited later
	for _, channel := range strings.Split(Env.GetOrEmpty("MSG_CHANNELS"), ",") {
		if channel != "" {
			switch channel[0] {
			case '+': // a filtered channel
				filterRequired = true
				fallthrough // run next case as well
			case '*': // an unfiltered channel
				awaitMsgAgents = append(awaitMsgAgents, newMsgAgent(channel[1:], channel[0] == '+')) // run async task (returns channel)
			default:
				panic(fmt.Sprintf("First char of channel ID %s must be * or +", channel))
			}
		}
	}

	// role init (requires dir)
	var awaitRoles = make([]chan (bool), 0)            // a list to collect async tasks, to be awaited later. there's only 1 but i might add more later
	if roleID = Env.GetOrEmpty("ROLE"); roleID != "" { // if ROLE is missing, user probs doesn't want a role
		roleServerID = Env.GetOrExit("SERVER")      // if ROLE is there but SERVER missing, user probs forgot the server
		awaitRoles = append(awaitRoles, roleInit()) // run async task (returns channel)
	}

	// await parallel init tasks (by doing blocking reads on their returned channels; see rsp. functions)
	for _, awaitMsgAgent := range awaitMsgAgents {
		a := <-awaitMsgAgent // the task will bear an agent a
		msgAgents = append(msgAgents, a)
		log.Insta <- fmt.Sprintf("m%d| started %s-%t", a.ID, a.channelID, a.filtered)
	}
	for _, awaitRole := range awaitRoles {
		<-awaitRole
	}
	log.Insta <- ". | initialised\n"
}

// main function (infinite loop)
func main() {
	for {
		new, err := fetch() // synchronous Twitch http call
		if err == nil {
			log.Bkgd <- fmt.Sprintf("< | %s", time.Now().Format("15:04:05"))
			var newFiltered map[string]*stream // declare a map to subset "new" on known/filtered users
			if filterRequired {
				newFiltered = make(map[string]*stream) // init said map cos we need it
				for user, stream := range new {
					if stream.filter >= 1 {
						newFiltered[user] = stream
					}
				}
			}
			for _, a := range msgAgents {
				if a.filtered { // the agents run msg(), a permanent worker coroutine thread that awaits on these channels
					a.inCh <- newFiltered
				} else {
					a.inCh <- new
				}
			}
			if roleID != "" {
				go role(new) // async call to role(), runs as a one-off task (no return)
			}
			time.Sleep(60 * time.Second)
		} else {
			time.Sleep(15 * time.Second)
		}
	}
}
