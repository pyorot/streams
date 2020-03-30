package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"

	"github.com/bwmarrin/discordgo"
)

// runs a single co-routine thread to process incoming info and synchronously send Discord commands

type msgsEntry struct {
	stream *stream // streams object (has info on stream)
	msgID  string  // the ID of the Discord message we're managing to represent this stream
}

type command struct { // represents an action to be done on Discord
	action rune    // 'a': add stream; 'e': edit stream info; 'r': remove stream
	user   string  // stream username
	stream *stream // stream object
}

type msgAgent struct {
	inCh      chan (map[string]*stream)
	msgs      map[string]*msgsEntry
	channelID string
	filtered  bool
}

func newMsgAgent(channelID string, filtered bool) chan (map[string]*stream) {
	a := &msgAgent{
		inCh:      make(chan map[string]*stream),
		msgs:      make(map[string]*msgsEntry, 40),
		channelID: channelID,
		filtered:  filtered,
	}
	a.init()
	go a.run()
	return a.inCh
}

var msgChs = make([]chan (map[string]*stream), 0)

var iconURLFail string
var iconURLPass string
var iconURLKnown string

// blocking http req to reload msg state from the msg Discord channel on startup
func (a *msgAgent) init() {
	// load message history
	history, err := discord.ChannelMessages(a.channelID, 50, "", "", "") // 50 msgs
	exitIfError(err)
	// pick msgs that we'd been managing on last shutdown; decode + register them
	for _, msg := range history {
		if len(msg.Embeds) == 1 && msg.Embeds[0].Color == 0x00ff00 { // pick green msgs with 1 embed
			user := msg.Embeds[0].Author.Name[:strings.IndexByte(msg.Embeds[0].Author.Name, ' ')] // first word in title
			startTime, err := time.Parse("2006-01-02T15:04:05-07:00", msg.Embeds[0].Timestamp)
			exitIfError(err)
			stream := stream{
				UserName:  user,
				Title:     msg.Embeds[0].Description[1:strings.IndexByte(msg.Embeds[0].Description, ']')],
				StartedAt: startTime,
			}
			a.msgs[strings.ToLower(user)] = &msgsEntry{&stream, msg.ID} // register stream decoded from msg
		}
	}
	log.Insta <- fmt.Sprintf("m | init [%d]", len(a.msgs))
}

// the message-managing co-routine
func (a *msgAgent) run() {
	for {
		// generate command queue from new data
		new := <-a.inCh                // input
		commands := make([]command, 0) // output
		for user := range a.msgs {     // iterate thru old to pick removals
			_, isInNew := new[user]
			if !isInNew { // remove
				commands = append(commands, command{'r', user, nil})
				log.Insta <- "m | - " + user
			}
		}
		for user := range new { // iterate thru new to pick edits + adds
			streamNew := new[user]
			_, isInOld := a.msgs[user]
			if isInOld && streamNew.Title != a.msgs[user].stream.Title { // edit if title changed
				commands = append(commands, command{'e', user, streamNew})
				log.Insta <- "m | ~ " + user
			} else if !isInOld { // add
				commands = append(commands, command{'a', user, streamNew})
				log.Insta <- "m | + " + user
			}
		}

		// process command queue (all commands are synchronous)
		// the msgs in Discord are colour-coded: yellow = being created; green = active stream; red = ended stream
		for _, cmd := range commands {
			switch cmd.action {
			case 'a': // will create new msg, then edit in info (to avoid losing a duplicate if it fails)
				msgID := a.msgAdd()                              // create new blank (yellow) msg
				a.msgEdit(msgID, cmd.stream, true)               // edit it to current info (turns green)
				a.msgs[cmd.user] = &msgsEntry{cmd.stream, msgID} // register msg
			case 'e':
				a.msgEdit(a.msgs[cmd.user].msgID, cmd.stream, true) // edit existing msg to current info
				a.msgs[cmd.user].stream = cmd.stream                // update stream object (pointer)
			case 'r': // will swap its msg with oldest green msg (keeps greens grouped at bottom), then turns it red
				// first, find ID of oldest green msg
				user, msgID := cmd.user, a.msgs[cmd.user].msgID // msg being closed
				minUser, minID := user, msgID                   // oldest green msg will go here
				for userTest := range a.msgs {                  // find lexicographic min msg ID (lower ID = older msg)
					if strings.Compare(a.msgs[userTest].msgID, minID) == -1 {
						minUser, minID = userTest, a.msgs[userTest].msgID
					}
				}
				// then do swaps + updates
				if minID != msgID { // if a swap even needs to be done
					log.Insta <- "m | " + user + " ↔ " + minUser
					a.msgs[user].msgID, a.msgs[minUser].msgID = minID, msgID       // swap in internal state
					a.msgEdit(a.msgs[minUser].msgID, a.msgs[minUser].stream, true) // edit newer msg (now of an open stream)
				}
				a.msgEdit(a.msgs[user].msgID, a.msgs[user].stream, false) // edit (older) msg (now of a closed stream)
				delete(a.msgs, user)                                      // dereference in internal state
			}
		}
		log.Bkgd <- fmt.Sprintf("m | ok [%d]", len(a.msgs))
	}
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
func (a *msgAgent) msgEdit(msgID string, stream *stream, active bool) {
	for {
		_, err := discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: a.channelID,
			ID:      msgID,
			Embed:   generateMsg(stream, active),
		})
		time.Sleep(time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			log.Insta <- fmt.Sprintf("x | m~: %s", err)
		} else {
			return
		}
	}
}

// formats a stream into a message embed
func generateMsg(s *stream, live bool) *discordgo.MessageEmbed {
	var colour int
	var postText, thumbnail string
	var URL string = "https://twitch.tv/" + s.UserName
	if live {
		colour = 0x00ff00 // green
		postText = " is live"
		if len(s.ThumbnailURL) >= 20 { // blunt safety check; need to replace end of url with numbers
			thumbnail = s.ThumbnailURL[:len(s.ThumbnailURL)-20] + "440x248.jpg"
		}
	} else {
		colour = 0xff0000 // red
		postText = " was live"
	}
	_, isReg := twicord[strings.ToLower(s.UserName)]
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    s.UserName + postText,
			URL:     URL,
			IconURL: fmt.Sprintf("http://icons.iconarchive.com/icons/ph03nyx/super-mario/256/Hat-%s-icon.png", ifThenElse(isReg, "Mario", "Wario")),
		},
		Description: fmt.Sprintf("[%s](%s)", s.Title, URL),
		Color:       colour,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: thumbnail},
		Timestamp:   s.StartedAt.Format("2006-01-02T15:04:05Z"),
	}
}
