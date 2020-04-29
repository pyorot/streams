package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"
	"github.com/bwmarrin/discordgo"
	"github.com/nicklaw5/helix"
)

type stream struct {
	user      string    // Twitch handle
	title     string    // stream title
	start     time.Time // stream started at
	thumbnail string    // stream thumbnail URL
	filter    int       // 2 if user in Twicord; 1 if title tag/keyword match; 0 otherwise
}

// called only in fetch() to generate streams from incoming new data
func newStreamFromTwitch(r *helix.Stream) *stream {
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

// called only in msgAgent.init() to generate streams from persisted data
func newStreamFromMsg(msg *discordgo.Message) *stream {
	var s stream
	s.user = msg.Embeds[0].Author.Name[:strings.IndexByte(msg.Embeds[0].Author.Name, ' ')] // first word in title
	s.title = msg.Embeds[0].Description[1:strings.IndexByte(msg.Embeds[0].Description, ']')]
	s.start, err = time.Parse("2006-01-02T15:04:05-07:00", msg.Embeds[0].Timestamp)
	exitIfError(err)
	if msg.Embeds[0].Thumbnail != nil {
		s.thumbnail = msg.Embeds[0].Thumbnail.URL
	}
	// doesn't matter if wrong filter value; only needs to match icon
	for i, URL := range iconURL {
		if URL == msg.Embeds[0].Author.IconURL {
			s.filter = i
		}
	}
	return &s
}

// formats a stream into a message embed
// called only in msgEdit to generate messages
func newMsgFromStream(s *stream, live bool) *discordgo.MessageEmbed {
	var colour int
	var postText, thumbnail string
	var URL = "https://twitch.tv/" + s.user
	if live {
		colour = 0x00ff00 // green
		postText = " is live"
		thumbnail = s.thumbnail
	} else {
		colour = 0xff0000 // red
		postText = " was live"
	}
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    s.user + postText,
			URL:     URL,
			IconURL: iconURL[s.filter],
		},
		Description: fmt.Sprintf("[%s](%s)", s.title, URL),
		Color:       colour,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: thumbnail},
		Timestamp:   s.start.Format("2006-01-02T15:04:05Z"),
	}
}

// called only in newStreamFromTwitch – the filter is only run on incoming data
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
