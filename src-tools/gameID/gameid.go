// standalone tool to translate Twitch game IDs/names
// run in folder with .env file

package main

import (
	"fmt"
	"os"

	. "github.com/Pyorot/streams/utils"
	"github.com/nicklaw5/helix"
)

var err error            // placeholder error
var twitch *helix.Client // Twitch client
var input helix.GamesParams

func init() {
	// argument validation
	if len(os.Args) != 3 {
		exit()
	}
	input = helix.GamesParams{}
	if os.Args[1] == "i" {
		input.IDs = []string{os.Args[2]}
	} else if os.Args[1] == "n" {
		input.Names = []string{os.Args[2]}
	} else {
		exit()
	}
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
}

func main() {
	res, err := twitch.GetGames(&input)
	ExitIfError(err)
	if len(res.Data.Games) >= 1 {
		fmt.Printf("%s \"%s\"\n", res.Data.Games[0].ID, res.Data.Games[0].Name)
	} else {
		fmt.Println("Game not found.")
	}
}

func exit() {
	fmt.Println("Usage:\n./gametool i 69\n./gametool n \"Crash Bandicoot: The Wrath of Cortex\"")
	os.Exit(0)
}
