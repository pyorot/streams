package main

import (
	"fmt"
	"strings"
	"time"

	. "github.com/Pyorot/streams/utils"

	"github.com/nicklaw5/helix"
)

var getStreamsParams helix.StreamsParams // the const argument for getStreams calls, initialised in main.go:init()
var authed bool                          // is current auth token believed to be valid?

// blocking http request to Twitch getStreams
func fetch() (map[string]*stream, error) {
	auth()                                           // renew auth token if required
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
		authed = !(res != nil && res.StatusCode == 401) // trigger re-auth next run iff last error was 401 (deref ptr first!)
		Log.Insta <- fmt.Sprintf("x | < : %s", err)
	}
	return dict, err
}

// blocking http request to renew Twitch auth token if required, retry until success
func auth() {
	for !authed {
		res, err := twitch.GetAppAccessToken(nil) // make api call
		if err == nil {
			twitch.SetAppAccessToken(res.Data.AccessToken)
			authed = true
			Log.Insta <- fmt.Sprintf("< | a: %s", res.Data.AccessToken)
		} else {
			Log.Insta <- fmt.Sprintf("x | <a : %s", err)
			time.Sleep(20 * time.Second)
		}
	}
}
