// tool to translate Twitch game IDs/names
package main

import (
	"fmt"
	"os"

	. "github.com/Pyorot/streams/utils"
	"github.com/nicklaw5/helix"
)

func main() {
	if len(os.Args) != 3 {
		exit()
	}
	input := helix.GamesParams{}
	if os.Args[1] == "i" {
		input.IDs = []string{os.Args[2]}
	} else if os.Args[1] == "n" {
		input.Names = []string{os.Args[2]}
	} else {
		exit()
	}
	Env.Load()
	twitch, err := helix.NewClient(&helix.Options{
		ClientID:     Env.GetOrExit("TWITCH_ID"),
		ClientSecret: Env.GetOrExit("TWITCH_SEC"),
	})
	ExitIfError(err)
	r1, err := twitch.GetAppAccessToken()
	ExitIfError(err)
	twitch.SetAppAccessToken(r1.Data.AccessToken)
	r2, err := twitch.GetGames(&input)
	ExitIfError(err)
	if len(r2.Data.Games) >= 1 {
		fmt.Printf("%s \"%s\"\n", r2.Data.Games[0].ID, r2.Data.Games[0].Name)
	} else {
		fmt.Println("Game not found.")
	}
}

func exit() {
	fmt.Println("Usage:\n./gametool i 69\n./gametool n \"Crash Bandicoot: The Wrath of Cortex\"")
	os.Exit(0)
}
