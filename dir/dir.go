package dir

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Pyorot/streams/log"
	. "github.com/Pyorot/streams/utils"
	"github.com/bwmarrin/discordgo"
)

// dir is used to look up Discord users to assign roles to + maybe to filter a msg channel
// the channel contains a bunch of posts in the format (where dui = Discord userID, tun = Twitch username):
// "dir<comment>\n<dui1>\s<tun1>\n<dui2>\s<tun2>\n..."

var discord *discordgo.Session                 // Discord session (managed at higher level)
var channel string                             // dir channel
var managed bool                               // manage dir (vs treating it as read-only)
var data map[string]string                     // map: twitch user -> Discord user ID
var lock sync.Mutex                            // data mutex
var msgTop string                              // current managed message
var addCh = make(chan (struct{ k, v string })) // channel for entries to be dynamically added to dir

// Init : starts the dir component
func Init(discord_ *discordgo.Session, channel_ string, managed_ bool) {
	discord, channel, managed = discord_, channel_, managed_
	if managed {
		go manage()
	}
	log.Insta <- fmt.Sprintf("d | init (channel: %s; managed: %t)", channel, managed)
	Load()
}

// Load : loads data from the dir channel and assigns it to data variable
func Load() {
	if managed {
		for discord.State.User == nil { // init await (required for step 2.2)
		}
	}
	// 1: load posts from channel
	history, err := discord.ChannelMessages(channel, 100, "", "", "") // get last 100 msgs (= max poss)
	ExitIfError(err)
	dataNew := make(map[string]string, 70)
	dataInv := make(map[string]string, 70) // ephemeral, just to check for duplicates
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
				// 2:1.2: add to dir (check for duplicates)
				_, existsK := dataNew[k]
				if existsK {
					log.Insta <- fmt.Sprintf("! | d: twitch user %s declared multiple times", k)
				}
				dataNew[k] = v
				_, existsV := dataInv[v]
				if existsV {
					log.Insta <- fmt.Sprintf("! | d: discord user %s declared multiple times", v)
				}
				dataInv[v] = k
			}
			ExitIfError(s.Err())
			// 2.2: assign last message
			msgID, err := strconv.ParseInt(msg.ID, 10, 64)
			ExitIfError(err)
			if discord.State.User != nil && msg.Author.ID == discord.State.User.ID && msgID > latestAutoMsgID {
				latestAutoMsgID, msgTop = msgID, msg.ID
			}
		}
	}
	// 3: commit to state
	lock.Lock()
	data = dataNew
	lock.Unlock()
	log.Insta <- fmt.Sprintf("d | loaded [%d]", len(data))
}

// Add :
func Add(k, v string) {
	if data[k] == v {
		return
	}
	lock.Lock()
	defer lock.Unlock()
	data[k] = v
	if managed {
		addCh <- struct{ k, v string }{k, v}
	}
}

// Get :
func Get(k string) string {
	lock.Lock()
	defer lock.Unlock()
	return data[k]
}

// Inverse :
func Inverse() map[string]string {
	inverseDir := make(map[string]string, len(data))
	for k, v := range data {
		inverseDir[v] = k
	}
	return inverseDir
}

func manage() {
	var err error
	var p struct{ k, v string }
	for {
		// 1: determine input (read in new or retry old)
		if p.k == "" { // p is blanked iff success
			p = <-addCh
		} else {
			time.Sleep(15 * time.Second)
		}
		msgTopCopy := msgTop
		var msg *discordgo.Message
		if msgTopCopy != "" {
			// 2.A.1: get managed message
			msg, err = discord.ChannelMessage(channel, msgTopCopy)
			if err != nil {
				if err.Error()[:8] == "HTTP 404" {
					msgTop = "" // signals new msg needs to be created
					log.Insta <- fmt.Sprintf("ds| renew: missing")
				} else {
					log.Insta <- fmt.Sprintf("x | ds?: %s", err)
				}
				continue
			}
			// 2.A.1: check edit fits in message
			if len(msg.Content)+len(p.v)+len(p.k)+2 >= 2000 {
				msgTop = "" // signals new msg needs to be created
				log.Insta <- fmt.Sprintf("ds| renew: capacity")
				continue
			}
		} else {
			// 2.B: post blank message
			msg, err = discord.ChannelMessageSend(channel, "dir")
			if err != nil {
				log.Insta <- fmt.Sprintf("x | ds+: %s", err)
				continue
			}
			msgTop, msgTopCopy = msg.ID, msg.ID
		}
		// 3: edit new data into message
		msg, err = discord.ChannelMessageEdit(channel, msgTopCopy,
			msg.Content+fmt.Sprintf("\n%s %s", p.v, p.k))
		if err != nil {
			log.Insta <- fmt.Sprintf("x | ds~: %s", err)
			continue
		}
		log.Insta <- fmt.Sprintf("ds| +: %s %s", p.v, p.k)
		p.k = "" // ack: p is processed
	}
}
