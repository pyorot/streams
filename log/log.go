package log

import (
	"fmt"
	"time"
)

// Insta : instant logging channel
var Insta = make(chan (string))

// Bkgd : accumulator logging channel
var Bkgd = make(chan (string))

func init() {
	go log()
}

func log() {
	var output string
	for {
		output = ""
	neutral:
		select {
		case item := <-Insta:
			fmt.Printf("%s\n", item)
		case item := <-Bkgd:
			output = item
			for {
				select {
				case item := <-Insta:
					fmt.Printf("%s\n", output)
					fmt.Printf("%s\n", item)
					break neutral
				case item := <-Bkgd:
					output += " || " + item
				case <-time.After(3 * time.Second):
					fmt.Printf("%s\n", output)
					break neutral
				}
			}
		}
	}
}
