package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var msgs = make(map[string]*msgsEntry, 60)
var msgCh = make(chan (*map[string]*stream))

type msgsEntry struct {
	stream *stream
	msgID  string
}

type command struct {
	action rune
	user   string
	stream *stream
}

func msgInit() chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		// load message history
		history, err := discord.ChannelMessages(env["CHANNEL_ID"], 50, "", "", "")
		exitIfError(err)
		// decode + register green messages
		for _, msg := range history {
			if len(msg.Embeds) == 1 && msg.Embeds[0].Color == 0x00ff00 {
				user := msg.Embeds[0].Title[:strings.IndexByte(msg.Embeds[0].Title, ' ')]
				startTime, err := time.Parse("2006-01-02T15:04:05-07:00", msg.Embeds[0].Timestamp)
				exitIfError(err)
				stream := stream{
					UserName:  user,
					Title:     msg.Embeds[0].Description,
					StartedAt: startTime,
				}
				msgs[strings.ToLower(user)] = &msgsEntry{&stream, msg.ID}
			}
		}
		fmt.Printf("m | init [%d]\n", len(msgs))
		res <- true
	}()
	return res
}

func msg() {
	for {
		new := *<-msgCh
		commands := make([]command, 0)

		// generate command queue from new data
		for user := range msgs {
			_, isInNew := new[user]
			if !isInNew { // remove
				commands = append(commands, command{'r', user, nil})
				fmt.Printf("m | - %s\n", user)
			}
		}
		for user := range new {
			streamNew := new[user]
			_, isInOld := msgs[user]
			if isInOld && streamNew.Title != msgs[user].stream.Title { // update
				commands = append(commands, command{'e', user, streamNew})
				fmt.Printf("m | ~ %s\n", user)
			} else if !isInOld { // add
				commands = append(commands, command{'a', user, streamNew})
				fmt.Printf("m | + %s\n", user)
			}
		}

		// process command queue
		for _, cmd := range commands {
			switch cmd.action {
			case 'a':
				msgID := msgAdd()
				msgEdit(msgID, cmd.stream, true)
				msgs[cmd.user] = &msgsEntry{cmd.stream, msgID}
			case 'e':
				msgEdit(msgs[cmd.user].msgID, cmd.stream, true)
				msgs[cmd.user].stream = cmd.stream
			case 'r':
				// calc swap ID of closing msg with oldest open msg (so open msgs stay grouped at bottom)
				user, msgID := cmd.user, msgs[cmd.user].msgID
				minUser, minID := user, msgID
				for userTest := range msgs {
					if strings.Compare(msgs[userTest].msgID, minID) == -1 {
						minUser, minID = userTest, msgs[userTest].msgID
					}
				}
				if minID != msgID {
					fmt.Printf("m | %s ↔ %s\n", user, minUser)
					msgs[user].msgID, msgs[minUser].msgID = minID, msgID
					msgEdit(msgs[minUser].msgID, msgs[minUser].stream, true)
				}
				msgEdit(msgs[user].msgID, msgs[user].stream, false)
				// update messages at new IDs
				delete(msgs, user)
			}
		}
		fmt.Printf("m | ok [%d]\n", len(msgs))
	}
}

func msgAdd() (msgID string) {
	for {
		msgOut, err := discord.ChannelMessageSendComplex(
			env["CHANNEL_ID"],
			&discordgo.MessageSend{Embed: &discordgo.MessageEmbed{Color: 0xffff00}},
		)
		time.Sleep(time.Second)
		if err != nil {
			fmt.Printf("x | m+: %s\n", err)
		} else {
			return msgOut.ID
		}
	}
}

func msgEdit(msgID string, stream *stream, active bool) {
	for {
		_, err := discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel: env["CHANNEL_ID"],
			ID:      msgID,
			Embed:   generateMsg(stream, active),
		})
		time.Sleep(time.Second)
		if err != nil {
			fmt.Printf("x | m~: %s\n", err)
		} else {
			return
		}
	}
}

func generateMsg(s *stream, live bool) *discordgo.MessageEmbed {
	var colour int
	var postText, thumbnail string
	if live {
		colour = 0x00ff00
		postText = " is live"
		if len(s.ThumbnailURL) >= 20 {
			thumbnail = s.ThumbnailURL[:len(s.ThumbnailURL)-20] + "440x248.jpg"
		}
	} else {
		colour = 0xff0000
		postText = " was live"
	}
	return &discordgo.MessageEmbed{
		Title:       s.UserName + postText,
		Description: s.Title,
		URL:         "https://twitch.tv/" + s.UserName,
		Color:       colour,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: thumbnail},
		Timestamp:   s.StartedAt.Format("2006-01-02T15:04:05Z"),
		Footer:      &discordgo.MessageEmbedFooter{IconURL: "https://www.mariowiki.com/images/thumb/b/be/SMS_Shine_Sprite_Artwork.png/805px-SMS_Shine_Sprite_Artwork.png"},
	}
}
