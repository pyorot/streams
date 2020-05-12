package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"

	"github.com/bwmarrin/discordgo"
)

// runs a single co-routine thread to process incoming info and synchronously send Discord commands

type msgAgent struct {
	inCh            chan (map[string]*stream)
	streamsLive     streamEntries
	streamsExpiring streamEntries
	channelID       string
	filtered        bool
}

type streamEntries map[string]*streamEntry
type streamEntry struct {
	stream *stream // streams object (has info on stream)
	msgID  string  // the ID of the Discord message we're managing to represent this stream
}

type command struct { // represents an action to be done on Discord
	action rune    // 'a': add stream; 'e': edit stream info; 'r': remove stream
	user   string  // stream username
	stream *stream // stream object
}

var msgAgents = make([]*msgAgent, 0)
var iconURL = make([]string, 3)

func newMsgAgent(channelID string, filtered bool) chan (*msgAgent) {
	res := make(chan (*msgAgent))
	go func() {
		a := &msgAgent{
			inCh:            make(chan map[string]*stream),
			streamsLive:     make(map[string]*streamEntry, 40),
			streamsExpiring: make(map[string]*streamEntry, 20),
			channelID:       channelID,
			filtered:        filtered,
		}
		a.init()
		go a.run()
		res <- a
	}()
	return res
}

// blocking http req to reload msg state from the msg Discord channel on startup
func (a *msgAgent) init() {
	// load message history
	history, err := discord.ChannelMessages(a.channelID, 50, "", "", "") // 50 msgs
	exitIfError(err)
	// pick msgs that we'd been managing on last shutdown; decode + register them
	for _, msg := range history {
		if len(msg.Embeds) == 1 { // pick msgs with 1 embed
			switch msg.Embeds[0].Color { // pick green and orange messages
			case embedColours[0]:
				s := newStreamFromMsg(msg)
				a.streamsLive[strings.ToLower(s.user)] = &streamEntry{s, msg.ID} // register stream decoded from msg
			case embedColours[1]:
				s := newStreamFromMsg(msg)
				a.streamsExpiring[strings.ToLower(s.user)] = &streamEntry{s, msg.ID} // register stream decoded from msg
			}
		}
	}
	log.Insta <- fmt.Sprintf("m | init [%d | %d]", len(a.streamsLive), len(a.streamsExpiring))
}

// the message-managing co-routine
func (a *msgAgent) run() {
	for {
		// generate command queue from new data
		streamsNew := <-a.inCh            // input
		commands := make([]command, 0)    // output
		for user := range a.streamsLive { // iterate thru old to pick removals
			_, isInNew := streamsNew[user]
			if !isInNew { // remove
				commands = append(commands, command{'r', user, nil})
			}
		}
		for user := range streamsNew { // iterate thru new to pick edits + adds
			_, isInOld := a.streamsLive[user]
			if isInOld && streamsNew[user].title != a.streamsLive[user].stream.title { // edit if title changed
				commands = append(commands, command{'e', user, streamsNew[user]})
			} else if !isInOld { // add
				commands = append(commands, command{'a', user, streamsNew[user]})
			}
		}

		// process command queue (all commands are synchronous)
		// the msgs in Discord are colour-coded: yellow = being created; green = active stream; red = ended stream
		for _, cmd := range commands {
			user, streamLatest := cmd.user, cmd.stream
			switch cmd.action {

			case 'a':
				_, exists := a.streamsExpiring[user]
				if !exists { // will create new msg, then edit in info (to avoid losing a duplicate if it fails)
					log.Insta <- "m | + " + user
					msgID := a.msgAdd()                                     // create new blank (yellow) msg
					a.streamsLive[user] = &streamEntry{streamLatest, msgID} // register msg
				} else { // will swap the old msg with newest orange msg (keeps greens grouped at bottom), then turns it green
					msgID := a.streamsExpiring[user].msgID
					maxUser, maxID := a.streamsExpiring.getExtremalEntry(+1) // find ID of newest orange msg
					log.Insta <- "m | * " + user + " ↔ " + maxUser           //
					if maxID != msgID {                                      // if a swap even needs to be done
						a.streamsExpiring[user].msgID, a.streamsExpiring[maxUser].msgID = maxID, msgID // swap in internal state
						a.msgEdit(a.streamsExpiring[maxUser], 1)                                       // edit newer msg (now of an open stream)
					}
					a.streamsLive[user] = a.streamsExpiring[user]         // move msg to live
					delete(a.streamsExpiring, user)                       //
					a.streamsLive[user].stream.title = streamLatest.title // take latest title
				}
				a.msgEdit(a.streamsLive[user], 0) // update msg (turns green)

			case 'e':
				log.Insta <- "m | ~ " + user
				a.streamsLive[user].stream.title = streamLatest.title // take latest title
				a.msgEdit(a.streamsLive[user], 0)                     // update msg

			case 'r': // will swap its msg with oldest green msg (keeps greens grouped at bottom), then turns it orange
				msgID := a.streamsLive[user].msgID
				minUser, minID := a.streamsLive.getExtremalEntry(-1) // find ID of oldest green msg
				log.Insta <- "m | - " + user + " ↔ " + minUser       //
				if minID != msgID {                                  // if a swap even needs to be done
					a.streamsLive[user].msgID, a.streamsLive[minUser].msgID = minID, msgID // swap in internal state
					a.msgEdit(a.streamsLive[minUser], 0)                                   // edit newer msg (now of an open stream)
				}
				a.streamsExpiring[user] = a.streamsLive[user]                                            // move msg to expiring                                    // move msg to expiring
				delete(a.streamsLive, user)                                                              //
				a.streamsExpiring[user].stream.length = time.Since(a.streamsExpiring[user].stream.start) // update stream length
				a.msgEdit(a.streamsExpiring[user], 1)                                                    // edit older msg (now of a closed stream)
			}
		}

		// manage expiries
		for user, se := range a.streamsExpiring {
			if s := se.stream; time.Since(s.start.Add(s.length)).Minutes() > 15 {
				log.Insta <- "m | / " + user
				delete(a.streamsExpiring, user)
				a.msgEdit(se, 2)
			}
		}

		log.Bkgd <- fmt.Sprintf("m | ok [%d]", len(a.streamsLive))
	}
}

func (m streamEntries) getExtremalEntry(sign int) (string, string) {
	var extUser, extID string
	for user := range m { // get a member (bound)
		extUser, extID = user, m[user].msgID
		break
	}
	for user := range m { // find lexicographic extremal msg ID (lower ID = older msg)
		if strings.Compare(m[user].msgID, extID) == sign {
			extUser, extID = user, m[user].msgID
		}
	}
	return extUser, extID
}

// blocking http req to post empty yellow msg (retry until successful); returns ID of new msg if successful
func (a *msgAgent) msgAdd() (msgID string) {
	for {
		msgOut, err := discord.ChannelMessageSendComplex(
			a.channelID,
			&discordgo.MessageSend{Embed: &discordgo.MessageEmbed{Color: 0xffff00}},
		)
		time.Sleep(time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			log.Insta <- fmt.Sprintf("x | m+: %s", err)
		} else {
			return msgOut.ID
		}
	}
}

// blocking http req to edit msg (retry until successful)
func (a *msgAgent) msgEdit(se *streamEntry, state int) {
	for {
		_, err := discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: a.channelID,
			ID:      se.msgID,
			Embed:   newMsgFromStream(se.stream, state),
		})
		time.Sleep(time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			log.Insta <- fmt.Sprintf("x | m~: %s", err)
		} else {
			return
		}
	}
}
