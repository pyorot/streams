package utils // an accumulator logger designed to log serially but cut down on lines

import (
	"fmt"
	"time"
)

type utilsLog struct{ Insta, Bkgd chan (string) }

// Log : utility functions for logging
var Log = utilsLog{
	Insta: make(chan string), // post here to instantly log message in new line (flushes buffer)
	Bkgd:  make(chan string), // post here to buffer message (flushed after 3s of inactivity, into a single line)
}

func init() {
	go log()
}

// worker thread picking up messages from insta and bkgd
func log() {
	var output string
	for { // outer loop
		output = ""
	neutral: // default state
		select { // await multiple channels and trigger on first received item
		case item := <-Log.Insta:
			fmt.Printf("%s\n", item)
		case item := <-Log.Bkgd:
			output = item // buffer a message in this variable
			for {         // inner loop
				select { // await same channels, but we have buffered messages to flush
				case item := <-Log.Insta: // immediately flush (and return to outer loop)
					fmt.Printf("%s\n", output)
					fmt.Printf("%s\n", item)
					break neutral
				case item := <-Log.Bkgd: // keep buffering (and return to inner loop)
					output += " || " + item
				case <-time.After(3 * time.Second): // if nothing received in 3s, flush
					fmt.Printf("%s\n", output)
					break neutral
				}
			}
		}
	}
}
