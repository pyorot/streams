package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/nicklaw5/helix"
)

type stream struct {
	user      string        // Twitch handle
	title     string        // stream title
	start     time.Time     // stream started at
	length    time.Duration // total stream length (including gaps)
	thumbnail string        // stream thumbnail URL
	filter    int           // 2 if user in Twicord; 1 if title tag/keyword match; 0 otherwise
}

var embedColours = [3]int{0x00ff00, 0xff8000, 0xff0000}

// called only in fetch() to generate streams from incoming new data
func newStreamFromTwitch(r *helix.Stream) *stream {
	lastHyphen := strings.LastIndexByte(r.ThumbnailURL, '-')
	s := &stream{
		user:      r.UserName,
		title:     r.Title,
		start:     r.StartedAt,
		thumbnail: r.ThumbnailURL[:lastHyphen+1] + "440x248.jpg",
		// length is not needed
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
	if msg.Embeds[0].Footer != nil && msg.Embeds[0].Footer.Text != "" {
		s.length, err = time.ParseDuration(msg.Embeds[0].Footer.Text)
		exitIfError(err)
	}
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
func newMsgFromStream(s *stream, state int) *discordgo.MessageEmbed {
	var URL = "https://twitch.tv/" + s.user
	var colour = embedColours[state]
	var postText, thumbnail string
	var length string
	if state == 0 {
		postText = " is live"
		thumbnail = s.thumbnail
	} else {
		postText = " was live"
		length = strings.TrimSuffix(s.length.Truncate(time.Minute).String(), "0s")
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
		Footer:      &discordgo.MessageEmbedFooter{Text: length},
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
			return true
		}
	}
	// else
	return false
}
