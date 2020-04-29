package main

import (
	"fmt"
	"strings"

	log "github.com/Pyorot/streams/log"
	"github.com/nicklaw5/helix"
)

var getStreamsParams helix.StreamsParams // the const argument for getStreams calls

// blocking http request to Twitch getStreams
func fetch() (map[string]*stream, error) {
	dict := make(map[string]*stream)                 // the return dict (twitch username → stream object)
	res, err := twitch.GetStreams(&getStreamsParams) //
	if err == nil && res.StatusCode != 200 {         // reinterpret HTTP error as actual error
		err = fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if err == nil {
		list := res.Data.Streams // result is in list format
		for i := range list {    // recompile into target dict format
			dict[strings.ToLower(list[i].UserName)] = newStreamFromTwitch(&list[i])
		}
	} else {
		log.Insta <- fmt.Sprintf("x | < : %s", err)
	}
	return dict, err
}
