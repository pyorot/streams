package log // an accumulator logger designed to log serially but cut down on lines

import (
	"fmt"
	"time"
)

// Insta : post here to instantly log message in new line (flushes buffer)
var Insta = make(chan (string))

// Bkgd : post here to buffer message (flushed after 3s of inactivity, into a single line)
var Bkgd = make(chan (string))

func init() {
	go log()
}

// worker thread picking up messages from Insta and Bkgd
func log() {
	var output string
	for { // outer loop
		output = ""
	neutral: // default state
		select { // await multiple channels and trigger on first received item
		case item := <-Insta:
			fmt.Printf("%s\n", item)
		case item := <-Bkgd:
			output = item // buffer a message in this variable
			for {         // inner loop
				select { // await same channels, but we have buffered messages to flush
				case item := <-Insta: // immediately flush (and return to outer loop)
					fmt.Printf("%s\n", output)
					fmt.Printf("%s\n", item)
					break neutral
				case item := <-Bkgd: // keep buffering (and return to inner loop)
					output += " || " + item
				case <-time.After(3 * time.Second): // if nothing received in 3s, flush
					fmt.Printf("%s\n", output)
					break neutral
				}
			}
		}
	}
}
