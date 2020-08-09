package main

import (
	"fmt"
	"strings"
	"time"

	. "github.com/Pyorot/streams/utils"

	"github.com/bwmarrin/discordgo"
	"github.com/nicklaw5/helix"
)

// represents a current stream, for both live updates and internal state
type stream struct {
	user      string        // Twitch handle (set on creation, not updated in internal state)
	title     string        // stream title (set on creation, updated)
	start     time.Time     // stream started at (set on creation, not updated)
	length    time.Duration // total stream length inc. gaps (not set on creation, updated on stream going offline)
	thumbnail string        // stream thumbnail URL (set on creation, not updated)
	filter    int           // 2 (user in Twicord); 1 (tag/keyword match); 0 (else) (set on creation, not updated)
}

var embedColours = [3]int{0x00ff00, 0xff8000, 0xff0000} // index = stream state: 0 (up); 1 (down, expiring); 2 (down, expired)

// called only in fetch() to generate live updates from incoming new data
func newStreamFromTwitch(r *helix.Stream) *stream {
	s := &stream{
		user:      r.UserName,
		title:     r.Title,
		start:     r.StartedAt,
		thumbnail: r.ThumbnailURL[:strings.LastIndexByte(r.ThumbnailURL, '-')+1] + "440x248.jpg",
		// length is not set until stream goes down
	}
	if _, isReg := dir[strings.ToLower(s.user)]; isReg {
		s.filter = 2
	} else if filterStream(r) {
		s.filter = 1
	} // else zero-initialised
	return s
}

// called only in msgAgent.init() to generate internal state from persisted data
// note: length calc (msg.run() remove) will be wrong if stream went down while program off
func newStreamFromMsg(msg *discordgo.Message) *stream {
	var s stream
	s.user = msg.Embeds[0].Author.Name[:strings.IndexByte(msg.Embeds[0].Author.Name, ' ')]   // first word in author
	s.title = msg.Embeds[0].Description[1:strings.IndexByte(msg.Embeds[0].Description, ']')] // "[user](link)" in description
	s.start, err = time.Parse("2006-01-02T15:04:05-07:00", msg.Embeds[0].Timestamp)
	ExitIfError(err)
	if msg.Embeds[0].Footer != nil && msg.Embeds[0].Footer.Text != "" {
		s.length, err = time.ParseDuration(msg.Embeds[0].Footer.Text) // relying on go default format
		ExitIfError(err)
	}
	if msg.Embeds[0].Thumbnail != nil {
		s.thumbnail = msg.Embeds[0].Thumbnail.URL
	}
	for i, URL := range iconURL { // will pick highest matching i; doesn't matter
		if msg.Embeds[0].Author.IconURL == URL {
			s.filter = i
		}
	}
	return &s
}

// called only in msgEdit to generate embeds for messages
func newMsgFromStream(s *stream, state int) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    s.user + IfThenElse(state == 0, " is live", " was live"),
			URL:     "https://twitch.tv/" + s.user,
			IconURL: iconURL[s.filter],
		},
		Description: fmt.Sprintf("[%s](%s)", s.title, "https://twitch.tv/"+s.user),
		Color:       embedColours[state],
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: IfThenElse(state == 0, s.thumbnail, "")},
		Footer:      &discordgo.MessageEmbedFooter{Text: IfThenElse(state == 0, "", strings.TrimSuffix(s.length.Truncate(time.Minute).String(), "0s"))},
		Timestamp:   s.start.Format("2006-01-02T15:04:05Z"),
	}
}

// called only in newStreamFromTwitch – the filter is run on incoming data and used only when a new msg is made
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
