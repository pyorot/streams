package dir

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"sync"

	. "github.com/Pyorot/streams/src/utils"

	"github.com/bwmarrin/discordgo"
)

// dir is used to look up Discord users to assign roles to + maybe to filter a msg channel
// the channel contains a bunch of posts in the format (where dui = Discord userID, tun = Twitch username):
// "dir<comment>\n<dui1>\s<tun1>\n<dui2>\s<tun2>\n..."

var discord *discordgo.Session // Discord session (managed at higher level)
var channel string             // dir channel
var data map[string]string     // map: twitch user -> Discord user ID
var blocks map[string]bool     // set: twitch user
var lock sync.Mutex            // mutex for data (blocks is only accessed in one thread)

// Init : async init of dir component
func Init(discord_ *discordgo.Session) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		discord = discord_
		channel, managed = Env.GetOrExit("DIR_CHANNEL"), Env.GetOrEmpty("DIR_MANAGED") == "true"
		if managed {
			gameName, serverID = Env.GetOrExit("GAME_NAME"), Env.GetOrExit("SERVER")
			go manage()                                                                      // start worker reading from addCh
			discord.AddHandler(add)                                                          // start callback posting to addCh
			discord.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildPresences) // 2020 api change: opt into events
			err := discord.Open()                                                            // start connection, trigger Ready event
			ExitIfError(err)
		}
		Load() // await Ready event, then load
		Log.Insta <- fmt.Sprintf("d | init [%d|%d] (%s-%-5t) (%s, %s)", len(data), len(blocks), channel, managed, serverID, gameName)
		res <- true
	}()
	return res
}

// Load : loads data from the dir channel and assigns it to data variable
func Load() {
	// 0: await init (required for step 2.2)
	if managed {
		for discord.State.User == nil {
		}
	}
	// 1: load posts from channel
	history, err := discord.ChannelMessages(channel, 100, "", "", "") // get last 100 msgs (= max poss)
	ExitIfError(err)
	dataNew := make(map[string]string, 70)
	dataInv := make(map[string]string, 70) // ephemeral, just to check for duplicates
	blocksNew := make(map[string]bool, 10)
	var latestAutoMsgID int64
	// 2: process each dir message
	for _, msg := range history {
		if len(msg.Content) >= 4 && msg.Content[:3] == "dir" {
			// 2.1: parse message (line-by-line)
			s := bufio.NewScanner(strings.NewReader(msg.Content)) // line iterator
			s.Scan()                                              // skip 1st line ("dir<comment>\n")
			for s.Scan() {
				// 2.1.1: parse line
				line := strings.TrimSpace(s.Text())
				splitIndex := strings.IndexByte(line, ' ')
				k, v := strings.ToLower(line[splitIndex+1:]), line[:splitIndex] // dicts are rhs → lhs
				// 2:1.2: add to dir table (check for duplicates)
				_, existsK := dataNew[k]
				if existsK {
					Log.Insta <- fmt.Sprintf("! | d: twitch user %s declared multiple times", k)
				}
				dataNew[k] = v
				_, existsV := dataInv[v]
				if existsV {
					Log.Insta <- fmt.Sprintf("! | d: discord user %s declared multiple times", v)
				}
				dataInv[v] = k
			}
			ExitIfError(s.Err())
			// 2.2: assign last message
			msgID, err := strconv.ParseInt(msg.ID, 10, 64)
			ExitIfError(err)
			if discord.State.User != nil && msg.Author.ID == discord.State.User.ID && msgID > latestAutoMsgID {
				latestAutoMsgID, manMsgID = msgID, msg.ID
			}
		} else if len(msg.Content) >= 6 && msg.Content[:5] == "block" {
			s := bufio.NewScanner(strings.NewReader(msg.Content)) // line iterator
			s.Scan()                                              // skip 1st line ("block<comment>\n")
			for s.Scan() {
				blocksNew[strings.ToLower(strings.TrimSpace(s.Text()))] = true
			}
			ExitIfError(s.Err())
		}
	}
	// 3: commit to state
	lock.Lock()
	data = dataNew
	blocks = blocksNew
	lock.Unlock()
	Log.Insta <- fmt.Sprintf("d | loaded [%d|%d]", len(data), len(blocks))
}

// Get :
func Get(k string) string {
	lock.Lock()
	defer lock.Unlock()
	return data[k]
}

// IsBlocked :
func IsBlocked(k string) bool {
	_, exists := blocks[k]
	return exists
}

// Inverse :
func Inverse() map[string]string {
	inverseDir := make(map[string]string, len(data))
	for k, v := range data {
		inverseDir[v] = k
	}
	return inverseDir
}
