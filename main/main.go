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

var err error                            // placeholder error
var twitch *helix.Client                 // Twitch client
var discord *discordgo.Session           // Discord client
var getStreamsParams helix.StreamsParams // the const argument for getStreams calls

var filterTags []string               // Twitch tags to look for
var filterKeywords []string           // Title keywords to look for
var twicordChannel string             // Channel to DL twicord data from
var twicord = make(map[string]string) // map: twitch user -> Discord user ID
// twicord is used to find Discord users to assign roles to + maybe to filter the msg channel

type stream helix.Stream

func getEnvOrExit(key string) string {
	val, exists := os.LookupEnv(key)
	if !exists {
		panic(fmt.Sprintf("Missing env var: %s", key))
	}
	return val
}

func getEnvOrEmpty(key string) string {
	val, _ := os.LookupEnv(key)
	return val
}

// runs on program start
func init() {
	var tasks = make([]chan (bool), 0)

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
	twitch, err = helix.NewClient(&helix.Options{ClientID: getEnvOrExit("TWITCH")})
	exitIfError(err)
	discord, err = discordgo.New("Bot " + getEnvOrExit("DISCORD"))
	exitIfError(err)
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{getEnvOrExit("GAME")}, // list of games to query
		First:   100,                            // maximum query results (limit is 100)
	}

	// filter init
	twicordChannel = getEnvOrEmpty("TWICORD_CHANNEL")
	filterTags = strings.Split(getEnvOrEmpty("FILTER_TAGS"), ",")
	filterKeywords = strings.Split(getEnvOrEmpty("FILTER_KEYWORDS"), ",")
	if twicordChannel != "" {
		tasks = append(tasks, twicordInit()) // run async task, which returns channel
	}
	log.Insta <- ". | Twicord channel: " + twicordChannel
	log.Insta <- fmt.Sprintf(". | Filter tags: %s", filterTags)
	log.Insta <- fmt.Sprintf(". | Filter keywords: %s", filterKeywords)

	// msg
	for _, channel := range strings.Split(getEnvOrEmpty("MSG_CHANNELS"), ",") {
		if len(channel) >= 1 {
			switch channel[0] {
			case '*':
				fallthrough
			case '+':
				msgChs = append(msgChs, newMsgAgent(channel[1:], channel[0] == '*'))
				log.Insta <- fmt.Sprintf("m | started %s", channel)
			default:
				panic(fmt.Sprintf("First char of channel ID %s must be * or +", channel))
			}
		}
	}

	// msg icons
	if url := getEnvOrEmpty("MSG_ICON"); url != "" {
		iconURLFail, iconURLPass, iconURLKnown = url, url, url
		if url2 := getEnvOrEmpty("MSG_ICON_PASS"); url2 != "" {
			iconURLPass, iconURLKnown = url2, url2
			if url3 := getEnvOrEmpty("MSG_ICON_KNOWN"); url3 != "" {
				iconURLKnown = url3
			}
		}
	}

	// role
	if roleID = getEnvOrEmpty("ROLE"); roleID != "" {
		roleServerID = getEnvOrExit("ROLE_SERVER")
		tasks = append(tasks, roleInit()) // run async task, which returns channel
	}

	// async parallel initialisation (see respective functions)
	for _, task := range tasks {
		<-task // await tasks doing blocking reads on them
	}

	log.Insta <- ". | initialised\n"
}

// main function (infinite loop)
func main() {
	for {
		new, err := fetch() // synchronous Twitch http call
		if err == nil {
			log.Bkgd <- fmt.Sprintf("< | %s", time.Now().Format("15:04:05"))
			for _, msgCh := range msgChs {
				msgCh <- new // post to msgCh, read by msg(), a permanent worker coroutine thread
			}
			if roleID != "" {
				go role(new) // async call to role(), runs as a task (no return)
			}
			time.Sleep(60 * time.Second)
		}
	}
}

// blocking http request to Twitch getStreams
func fetch() (map[string]*stream, error) {
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
		log.Insta <- fmt.Sprintf("x | < : %s", err)
	}
	return dict, err
}

// non-blocking http req to read twicord data from a Discord chan
// format is a sequence of posts in the format (where dui = Discord userID, tun = Twitch username):
// "twicord<comment>\n<dui1>\s<tun1>\n<dui2>\s<tun2>\n..."
func twicordInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		history, err := discord.ChannelMessages(twicordChannel, 20, "", "", "") // get last 20 msgs
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
		log.Insta <- fmt.Sprintf(". | twicord loaded [%d]", len(twicord))
		res <- true
	}()
	return res
}
