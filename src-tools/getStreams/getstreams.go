// standalone tool to show output of Twitch getStreams
// run in folder with .env file

package main

import (
	"fmt"

	. "github.com/Pyorot/streams/src/utils"

	"github.com/nicklaw5/helix"
)

var err error                            // placeholder error
var twitch *helix.Client                 // Twitch client
var getStreamsParams helix.StreamsParams // the const argument for getStreams calls, initialised in main.go:init()

func init() {
	// env vars
	Env.Load()
	// twitch
	twitch, err = helix.NewClient(&helix.Options{
		ClientID:     Env.GetOrExit("TWITCH_ID"),
		ClientSecret: Env.GetOrExit("TWITCH_SEC"),
	})
	ExitIfError(err)
	res, err := twitch.GetAppAccessToken(nil)
	ExitIfError(err)
	twitch.SetAppAccessToken(res.Data.AccessToken)
	getStreamsParams = helix.StreamsParams{
		GameIDs: []string{Env.GetOrExit("GAME_ID")}, // list of games to query
		First:   100,                                // maximum query results (limit is 100)
	}
	fmt.Println(". | init; press any key to get streams")
}

func main() {
	for {
		fmt.Scanln()
		res, err := twitch.GetStreams(&getStreamsParams)
		ExitIfError(err)
		for _, r := range res.Data.Streams {
			fmt.Printf("%6s, %11s, %9s, %s, %s, %s, %4d, %-20s,\n        %s,\n        %s,\n        %s\n",
				r.GameID, r.ID, r.UserID, r.Type, r.Language, r.StartedAt.Format("2006/01/02 15:04:05"), r.ViewerCount, r.UserName,
				r.Title,
				r.ThumbnailURL,
				r.TagIDs,
			)
		}
		if len(res.Data.Streams) == 100 {
			fmt.Println("! | limit of results entries (100) reached")
		}
		fmt.Println()
	}
}
