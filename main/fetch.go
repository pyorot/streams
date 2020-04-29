package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"
	"github.com/nicklaw5/helix"
)

var getStreamsParams helix.StreamsParams // the const argument for getStreams calls
var authExpiry time.Time                 // timepoint after which we must regenerate a token

// blocking http request to Twitch getStreams
func fetch() (map[string]*stream, error) {
	auth()                                           // check if auth token is still valid
	dict := make(map[string]*stream)                 // the return dict (twitch username → stream object)
	res, err := twitch.GetStreams(&getStreamsParams) //
	if err == nil && res.StatusCode != 200 {         // reinterpret HTTP error as actual error
		err = fmt.Errorf("HTTP %d: %s", res.StatusCode, res.ErrorMessage)
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

func auth() {
	if time.Now().After(authExpiry) {
		authExpiry = time.Now()
		res, err := twitch.GetAppAccessToken()
		exitIfError(err)
		twitch.SetAppAccessToken(res.Data.AccessToken)
		authExpiry = authExpiry.Add(time.Duration(res.Data.ExpiresIn) * time.Second)
		log.Insta <- fmt.Sprintf("< | token generated: %s (for %ds)", res.Data.AccessToken, res.Data.ExpiresIn)
	}
}
