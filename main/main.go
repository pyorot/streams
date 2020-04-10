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
var filterRequired bool               // will generate filtered map of incoming streams
var twicordChannel string             // Channel to DL twicord data from
var twicord = make(map[string]string) // map: twitch user -> Discord user ID
// twicord is used to find Discord users to assign roles to + maybe to filter the msg channel

// type stream helix.Stream

type stream struct {
	user      string    // Twitch handle
	title     string    // stream title
	start     time.Time // stream started at
	thumbnail string    // stream thumbnail URL
	filter    int       // 2 if user in Twicord; 1 if title tag/keyword match; 0 otherwise
}

func newStream(r *helix.Stream) *stream {
	lastHyphen := strings.LastIndexByte(r.ThumbnailURL, '-')
	if lastHyphen == -1 {
		log.Insta <- "x | invalid ThumbnailURL: " + r.ThumbnailURL
	}
	s := &stream{
		user:      r.UserName,
		title:     r.Title,
		start:     r.StartedAt,
		thumbnail: r.ThumbnailURL[:lastHyphen+1] + "440x248.jpg",
	}
	if _, isReg := twicord[strings.ToLower(s.user)]; isReg {
		s.filter = 2
	} else if filterStream(r) {
		s.filter = 1
	}
	return s
}

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
	var awaitTasks = make([]chan (bool), 0)
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
	if rawTags := getEnvOrEmpty("FILTER_TAGS"); rawTags != "" {
		filterTags = strings.Split(rawTags, ",")
	}
	if rawKeywords := getEnvOrEmpty("FILTER_KEYWORDS"); rawKeywords != "" {
		filterKeywords = strings.Split(rawKeywords, ",")
	}
	if twicordChannel != "" {
		awaitTasks = append(awaitTasks, twicordInit()) // run async task, which returns channel
	}
	log.Insta <- ". | Twicord channel: " + twicordChannel
	log.Insta <- fmt.Sprintf(". | Filter tags [%d]: %s", len(filterTags), filterTags)
	log.Insta <- fmt.Sprintf(". | Filter keywords [%d]: %s", len(filterKeywords), filterKeywords)

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
		awaitTasks = append(awaitTasks, roleInit()) // run async task, which returns channel
	}

	// async parallel initialisation (see respective functions)
	for _, awaitTask := range awaitTasks {
		<-awaitTask // await tasks doing blocking reads on them
	}
	for _, awaitMsgAgent := range awaitMsgAgents {
		a := <-awaitMsgAgent
		msgAgents = append(msgAgents, a)
		log.Insta <- fmt.Sprintf("m | started %s-%t", a.channelID, a.filtered)
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
			dict[strings.ToLower(list[i].UserName)] = newStream(&list[i])
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
					splitIndex := strings.IndexByte(line, ' ')                        // line is space-delimited
					twicord[strings.ToLower(line[splitIndex+1:])] = line[:splitIndex] // dict is rhs → lhs
				}
				exitIfError(scanner.Err())
			}
		}
		log.Insta <- fmt.Sprintf(". | twicord loaded [%d]", len(twicord))
		res <- true
	}()
	return res
}

func filterStream(r *helix.Stream) bool {
	// check tags
	for _, tag1 := range r.TagIDs {
		for _, tag2 := range filterTags {
			if tag1 == tag2 {
				return true
			}
		}
	}
	// check keywords
	title := strings.ToLower(r.Title)
	for _, keyword := range filterKeywords {
		if strings.Contains(title, keyword) {
			fmt.Printf("!{%s|%s}\n", title, keyword)
			return true
		}
	}
	// else
	return false
}
