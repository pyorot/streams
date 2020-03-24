package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// runs a single co-routine thread to process incoming info and synchronously send Discord commands

var msgs = make(map[string]*msgsEntry, 60)   // map of currently managed messages (user → {stream, msgID})
var msgCh = make(chan (*map[string]*stream)) // channel to receive input info from

type msgsEntry struct {
	stream *stream // streams object (has info on stream)
	msgID  string  // the ID of the Discord message we're managing to represent this stream
}

type command struct { // represents an action to be done on Discord
	action rune    // 'a': add stream; 'e': edit stream info; 'r': remove stream
	user   string  // stream username
	stream *stream // stream object
}

// non-blocking http req to reload msg state from the msg Discord channel on startup
func msgInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		// load message history
		history, err := discord.ChannelMessages(env["CHANNEL_ID"], 50, "", "", "") // 50 msgs
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
				msgs[strings.ToLower(user)] = &msgsEntry{&stream, msg.ID} // register stream decoded from msg
			}
		}
		fmt.Printf("m | init [%d]\n", len(msgs))
		res <- true
	}()
	return res
}

// the message-managing co-routine
func msg() {
	for {
		// generate command queue from new data
		new := *<-msgCh                // input
		commands := make([]command, 0) // output
		for user := range msgs {       // iterate thru old to pick removals
			_, isInNew := new[user]
			if !isInNew { // remove
				commands = append(commands, command{'r', user, nil})
				fmt.Printf("m | - %s\n", user)
			}
		}
		for user := range new { // iterate thru new to pick edits + adds
			streamNew := new[user]
			_, isInOld := msgs[user]
			if isInOld && streamNew.Title != msgs[user].stream.Title { // edit if title changed
				commands = append(commands, command{'e', user, streamNew})
				fmt.Printf("m | ~ %s\n", user)
			} else if !isInOld { // add
				commands = append(commands, command{'a', user, streamNew})
				fmt.Printf("m | + %s\n", user)
			}
		}

		// process command queue (all commands are synchronous)
		// the msgs in Discord are colour-coded: yellow = being created; green = active stream; red = ended stream
		for _, cmd := range commands {
			switch cmd.action {
			case 'a': // will create new msg, then edit in info (to avoid losing a duplicate if it fails)
				msgID := msgAdd()                              // create new blank (yellow) msg
				msgEdit(msgID, cmd.stream, true)               // edit it to current info (turns green)
				msgs[cmd.user] = &msgsEntry{cmd.stream, msgID} // register msg
			case 'e':
				msgEdit(msgs[cmd.user].msgID, cmd.stream, true) // edit existing msg to current info
				msgs[cmd.user].stream = cmd.stream              // update stream object (pointer)
			case 'r': // will swap its msg with oldest green msg (keeps greens grouped at bottom), then turns it red
				// first, find ID of oldest green msg
				user, msgID := cmd.user, msgs[cmd.user].msgID // msg being closed
				minUser, minID := user, msgID                 // oldest green msg will go here
				for userTest := range msgs {                  // find lexicographic min msg ID (lower ID = older msg)
					if strings.Compare(msgs[userTest].msgID, minID) == -1 {
						minUser, minID = userTest, msgs[userTest].msgID
					}
				}
				// then do swaps + updates
				if minID != msgID { // if a swap even needs to be done
					fmt.Printf("m | %s ↔ %s\n", user, minUser)
					msgs[user].msgID, msgs[minUser].msgID = minID, msgID     // swap in internal state
					msgEdit(msgs[minUser].msgID, msgs[minUser].stream, true) // edit newer msg (now of an open stream)
				}
				msgEdit(msgs[user].msgID, msgs[user].stream, false) // edit (older) msg (now of a closed stream)
				delete(msgs, user)                                  // dereference in internal state
			}
		}
		fmt.Printf("m | ok [%d]\n", len(msgs))
	}
}

// blocking http req to post empty yellow msg (retry until successful); returns ID of new msg if successful
func msgAdd() (msgID string) {
	for {
		msgOut, err := discord.ChannelMessageSendComplex(
			env["CHANNEL_ID"],
			&discordgo.MessageSend{Embed: &discordgo.MessageEmbed{Color: 0xffff00}},
		)
		time.Sleep(time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			fmt.Printf("x | m+: %s\n", err)
		} else {
			return msgOut.ID
		}
	}
}

// blocking http req to edit msg (retry until successful)
func msgEdit(msgID string, stream *stream, active bool) {
	for {
		_, err := discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: env["CHANNEL_ID"],
			ID:      msgID,
			Embed:   generateMsg(stream, active),
		})
		time.Sleep(time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			fmt.Printf("x | m~: %s\n", err)
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
