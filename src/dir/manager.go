package dir

import (
	"fmt"
	"strings"
	"time"

	. "github.com/Pyorot/streams/src/utils"

	"github.com/bwmarrin/discordgo"
)

var managed bool                               // manage dir (vs treating it as read-only)
var gameName string                            // (if managed) param for onUpdate
var serverID string                            // (if managed) param for onUpdate
var manMsgID string                            // current managed message
var addCh = make(chan (struct{ k, v string })) // channel connecting manage() and add()

// worker to read entries from addCh and process them
func manage() {
	var err error
	var p struct{ k, v string } // current entry (pair)
	for {
		// 1: determine input (read in new or retry old)
		if p.k == "" { // p.k is blanked iff success
			p = <-addCh
		} else {
			time.Sleep(15 * time.Second)
		}
		manMsgIDCopy := manMsgID // copy for concurrency coherency
		var msg *discordgo.Message
		if manMsgIDCopy != "" {
			// 2.A.1: get managed message
			msg, err = discord.ChannelMessage(channel, manMsgIDCopy)
			if err != nil {
				if err.Error()[:8] == "HTTP 404" {
					manMsgID = "" // signals new msg needs to be created
					Log.Insta <- fmt.Sprintf("d | renew - missing")
				} else {
					Log.Insta <- fmt.Sprintf("x | d?: %s", err)
				}
				continue
			}
			// 2.A.1: check edit fits in message
			if len(msg.Content)+len(p.v)+len(p.k)+2 >= 2000 {
				manMsgID = "" // signals new msg needs to be created
				Log.Insta <- fmt.Sprintf("d | renew - capacity")
				continue
			}
		} else {
			// 2.B: post blank message
			msg, err = discord.ChannelMessageSend(channel, "dir")
			if err != nil {
				Log.Insta <- fmt.Sprintf("x | d+: %s", err)
				continue
			}
			manMsgID, manMsgIDCopy = msg.ID, msg.ID
		}
		// 3: edit new data into message
		text := msg.Content + fmt.Sprintf("\n%s %s", p.v, p.k)
		msg, err = discord.ChannelMessageEdit(channel, manMsgIDCopy, text)
		if err != nil {
			Log.Insta <- fmt.Sprintf("x | d~: %s", err)
			continue
		}
		Log.Insta <- fmt.Sprintf("d | > %s %s", p.v, p.k)
		p.k = "" // ack (got to the end): p is processed
	}
}

// callback to post entries to addCh from WebSocket events
func add(s *discordgo.Session, pu *discordgo.PresenceUpdate) {
	for _, a := range pu.Activities {
		filter := pu.GuildID == serverID &&
			a.Name == "Twitch" &&
			a.Type == discordgo.GameTypeStreaming &&
			a.State == gameName
		if filter {
			k, v := a.URL[strings.LastIndex(a.URL, "/")+1:], pu.User.ID
			lock.Lock()
			defer lock.Unlock()
			if data[k] != v { // add only if new
				data[k] = v
				if managed {
					addCh <- struct{ k, v string }{k, v}
				}
			}
		}
	}
}
