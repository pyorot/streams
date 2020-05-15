package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Pyorot/streams/log"
	"github.com/nicklaw5/helix"
)

var getStreamsParams helix.StreamsParams // the const argument for getStreams calls, initialised in main.go:init()
var authExpiry time.Time                 // timepoint after which we must regenerate an auth token

// blocking http request to Twitch getStreams
func fetch() (map[string]*stream, error) {
	auth()                                           // check if auth token is still valid
	dict := make(map[string]*stream)                 // the return dict (twitch username → stream object)
	res, err := twitch.GetStreams(&getStreamsParams) // make api call
	if err == nil && res.StatusCode != 200 {         // reinterpret HTTP error as actual error
		err = fmt.Errorf("HTTP %d: %s", res.StatusCode, res.ErrorMessage)
	}
	if err == nil {
		list := res.Data.Streams // result is in list format
		for i := range list {    // recompile into target dict format with custom stream structs
			dict[strings.ToLower(list[i].UserName)] = newStreamFromTwitch(&list[i])
		}
	} else {
		log.Insta <- fmt.Sprintf("x | < : %s", err)
	}
	return dict, err
}

// blocking http request (if required) to Twitch auth to get (expiring) token
func auth() {
	if time.Now().After(authExpiry) {
		authExpiry = time.Now()                // take time before request to guarantee safe expiry window
		res, err := twitch.GetAppAccessToken() // make api call
		exitIfError(err)
		twitch.SetAppAccessToken(res.Data.AccessToken)
		authExpiry = authExpiry.Add(time.Duration(res.Data.ExpiresIn) * time.Second)
		log.Insta <- fmt.Sprintf("< | token generated: %s (for %ds)", res.Data.AccessToken, res.Data.ExpiresIn)
	}
}
