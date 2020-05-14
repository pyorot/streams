package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/nicklaw5/helix"
)

// main.go: main program loop + Twitch data fetching
// msg.go:  managing a streams channel (posting to Discord)
// role.go: managing a streams role (posting to Discord)

var err error                  // placeholder error
var twitch *helix.Client       // Twitch client
var discord *discordgo.Session // Discord client

var filterRequired bool               // will generate filtered map of incoming streams
var filterTags []string               // Twitch tags to look for
var filterKeywords []string           // Title keywords to look for
var twicord = make(map[string]string) // map: twitch user -> Discord user ID
// twicord is used to find Discord users to assign roles to + maybe to filter the msg channel

// runs on program start
func init() {
	var awaitRoles = make([]chan (bool), 0)
	var awaitMsgAgents = make([]chan (*msgAgent), 0)

	// load env vars from .env file if present
	err := godotenv.Load()
	if err == nil {
		log.Insta <- ". | Env vars loaded from .env"
	} else if os.IsNotExist(err) {
		log.Insta <- ". | Env vars pre-loaded"
	} else {
		panic(err)
	}

	// core init
	twitch, err = helix.NewClient(&helix.Options{
		ClientID:     getEnvOrExit("TWITCH_ID"),
		ClientSecret: getEnvOrExit("TWITCH_SEC"),
	})
	exitIfError(err)
	discord, err = discordgo.New("Bot " + getEnvOrExit("DISCORD"))
	exitIfError(err)
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{getEnvOrExit("GAME")}, // list of games to query
		First:   100,                            // maximum query results (limit is 100)
	}

	// filter init
	if rawTags := getEnvOrEmpty("FILTER_TAGS"); rawTags != "" {
		filterTags = strings.Split(rawTags, ",")
		log.Insta <- fmt.Sprintf(". | filter tags [%d]: %s", len(filterTags), filterTags)
	}
	if rawKeywords := getEnvOrEmpty("FILTER_KEYWORDS"); rawKeywords != "" {
		filterKeywords = strings.Split(rawKeywords, ",")
		log.Insta <- fmt.Sprintf(". | filter keywords [%d]: %s", len(filterKeywords), filterKeywords)
	}
	if twicordChannel := getEnvOrEmpty("TWICORD_CHANNEL"); twicordChannel != "" {
		twicordInit(twicordChannel)
	}

	// msg icons
	if url := getEnvOrEmpty("MSG_ICON"); url != "" {
		iconURL[0], iconURL[1], iconURL[2] = url, url, url
		if url2 := getEnvOrEmpty("MSG_ICON_PASS"); url2 != "" {
			iconURL[1], iconURL[2] = url2, url2
			if url3 := getEnvOrEmpty("MSG_ICON_KNOWN"); url3 != "" {
				iconURL[2] = url3
			}
		}
	}

	// msg agents (requires msg icons)
	for _, channel := range strings.Split(getEnvOrEmpty("MSG_CHANNELS"), ",") {
		if len(channel) >= 1 {
			switch channel[0] {
			case '+':
				filterRequired = true
				fallthrough
			case '*':
				awaitMsgAgents = append(awaitMsgAgents, newMsgAgent(channel[1:], channel[0] == '+'))
			default:
				panic(fmt.Sprintf("First char of channel ID %s must be * or +", channel))
			}
		}
	}

	// role
	if roleID = getEnvOrEmpty("ROLE"); roleID != "" {
		roleServerID = getEnvOrExit("ROLE_SERVER")
		awaitRoles = append(awaitRoles, roleInit()) // run async task, which returns channel
	}

	// async parallel initialisation (see respective functions)
	for _, awaitRole := range awaitRoles {
		<-awaitRole // await tasks by doing blocking reads on them
	}
	for _, awaitMsgAgent := range awaitMsgAgents {
		a := <-awaitMsgAgent
		msgAgents = append(msgAgents, a)
		log.Insta <- fmt.Sprintf("m%d| started %s-%t", a.ID, a.channelID, a.filtered)
	}
	log.Insta <- ". | initialised\n"
}

// main function (infinite loop)
func main() {
	for {
		new, err := fetch() // synchronous Twitch http call
		if err == nil {
			log.Bkgd <- fmt.Sprintf("< | %s", time.Now().Format("15:04:05"))
			var newFiltered map[string]*stream
			if filterRequired {
				newFiltered = make(map[string]*stream)
				for user, stream := range new {
					if stream.filter >= 1 {
						newFiltered[user] = stream
					}
				}
			}
			for _, a := range msgAgents {
				if a.filtered {
					a.inCh <- newFiltered
				} else {
					a.inCh <- new // post to msgCh, read by msg(), a permanent worker coroutine thread
				}
			}
			if roleID != "" {
				go role(new) // async call to role(), runs as a task (no return)
			}
			time.Sleep(60 * time.Second)
		}
	}
}

// blocking http req to read twicord data from a Discord chan
// format is a sequence of posts in the format (where dui = Discord userID, tun = Twitch username):
// "twicord<comment>\n<dui1>\s<tun1>\n<dui2>\s<tun2>\n..."
func twicordInit(channel string) {
	history, err := discord.ChannelMessages(channel, 20, "", "", "") // get last 20 msgs
	exitIfError(err)
	for _, msg := range history {
		if len(msg.Content) >= 8 && msg.Content[:7] == "twicord" { // pick msgs starting with "twicord"
			scanner := bufio.NewScanner(strings.NewReader(msg.Content)) // line-by-line iterator
			scanner.Scan()                                              // skip 1st line ("twicord<comment>\n")
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				splitIndex := strings.IndexByte(line, ' ')                        // line is space-delimited
				twicord[strings.ToLower(line[splitIndex+1:])] = line[:splitIndex] // dict is rhs → lhs
			}
			exitIfError(scanner.Err())
		}
	}
	log.Insta <- fmt.Sprintf(". | twicord loaded (from %s) [%d]", channel, len(twicord))
}
