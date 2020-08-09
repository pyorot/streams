# Design
A scrappy first-draft of how this program (v2) works.

## Files
* **main/fetch.go** – Twitch API routines to `auth()` and `fetch()` data
* **main/main.go** – core init and worker routines + dir init
* **main/msg.go** – Discord message channel init, worker, API methods
* **main/role.go** – Discord role init, execution task, API methods
* **main/stream.go** – streams struct with conversion methods + filter
* **main/utils.go** – misc macros and tools
* **log/log.go** – accumulator logger that exposes 2 channels for logging (see source)

## Streams Struct
This struct (defined in streams.go) and its conversions are the core data flow in the program. It represents both the snapshot of a stream on twitch, and the representation of a stream we know about in msg.go internal state (as a `streamEntry`).

**Conversion methods:**
* **newStreamFromTwitch()**: generates a `stream` from incoming live data (a snapshot).
* **newStreamFromMsg()**: generates a `stream` from persisted data in a Discord message (for a `streamEntry`).
* **newMsgFromStream()**: generates an updated Discord message from a `stream` (in a `streamEntry`); doesn't mutate stream object.

**Data transitions r.e. messages:**
* **user, start, thumbnail**: these 3 properties are set from incoming data in `newStreamFromTwitch()`, and never modified. The state stream copies the first snapshot during an add, then isn't touched, and gets encoded to + decoded from Discord messages.
* **filter**: this is calculated from incoming data by checking dir and running `filter()` on the title, but otherwise behaves as user/start/thumbnail. This means once the first snapshot enters internal state after an add, it will never change. However, filtered/unfiltered channels see different streams of snapshots (based on checking the filter), so messages representing the same stream may still differ between the two.
* **title**: this is set in an incoming snapshot or persisted message from the relevant read data, then every command other than remove updates it in internal state to match the latest snapshot.
* **length**: this is not set in an incoming snapshot, read if it exists from a persisted message, and otherwise *only* calculated (from the state stream's start) and set during a remove.

Another way of saying this: where:
* **r** – read from msg/fetched data
* **0** – not set (which means =0 in Go)
* **c** – calculate
* **u** – update state stream from snapshot stream

we have:
| . | newStreamFromMsg() | newStreamFromTwitch() | add (new) | add (from expiring) | edit | delete |
| -         | - | - | - | - | - | - |
| user      | r | r | u | - | - | - |
| start     | r | r | u | - | - | - |
| thumbnail | r | r | u | - | - | - |
| filter    | r | c | u | - | - | - |
| title     | r | r | u | u | u | - |
| length    | r | 0 | u | - | - | c |

## Live
**main**  
The bot runs a `main()` loop, which sync pulls a snapshot of streams data for the specified game, then distributes it to instances of `msg()` and `role()` in parallel, sleeping for a min before going again. Filtered msg channels receive a filtered snapshot, so have a self-contained view of what's happening to the game's streams.

**msg**  
A `msgAgent` represents a single message channel, and has its own (Go) channel to receive data, state representing what streams it knows about, and worker thread executing its `run()` looping method.

The state is 2 maps of Twitch handle → `streamEntry`, one covering *live* streams, and the other covering *expiring* streams that recently (within 15m) went down. A `streamEntry` is a `stream` (as per streams.go) and a message in the Discord channel representing it.

`run()` reads in a snapshot of current streams, then compares this to its state, issuing a list of add/edit/remove commands per user. These are then synchronously processed (retry until success), updating managed Discord messages via API calls, as well as its state. Then it can read the next input.

The API methods are:
* `msgAdd`: posts a blank msg and returns its ID
* `msgEdit(streamEntry, state)`: updates the msg belonging to streamEntry with the stream belonging to it, with state telling it if it's up/expiring/expired.

The commands work as follows. Refer to the streamEntry belonging to the user being processed as "self".
* add:
	* if user is in *expiring* streams,
		* get the newest *expiring* msgEntry ("other") and swap the two msg IDs in the *expiring* map
		* msgEdit "other" (state = expiring)
		* move "self" from *expiring* to *live*
		* data: set "self" stream title to incoming snapshot title
		* msgEdit "self" (state = up)
	* else
		* msgAdd (returns msgID)
		* data: add new streamEntry "self" to *live*, copying incoming snapshot, adding msgID
		* msgEdit "self" (state = up)
* edit:
	* data: set "self" stream title to incoming snapshot title
	* msgEdit "self" (state = up)
* remove:
	* get the oldest *live* msgEntry ("other") and swap the two msg IDs in the *live* map
	* msgEdit "other" (state = up)
	* move "self" from *live* to *expiring*
	* data: calculate + set "self" length to duration from "self" stream start to now
	* msgEdit "self" (state = expiring)

Finally, the end of `run()` checks every entry in *expiring* for if its start + length (= end) is 15 mins ago, and if so:
* delete from *expiring*
* msgEdit "self" (state = expired)

**role**  
`role()` is an async task, like a JS promise. It gets passed incoming streams data, compares it to its cached view of who's online, then issues in parallel all of its commands, awaiting confirmation before updating its state, then returning (if error, state isn't updated, so the command is dropped until next run).

## Persistence
Any changes to or recovery of the bot are done by restarting it, at any time. It recovers its state like this:

**msg**:  
`msgAgent.init()` looks at the last 50 messages in its Discord channel and takes ownership of any representing active streams, reading info about a stream from its message into the state.

**role**:  
`roleInit()` creates a one-off inverted dir, then goes through the entire user-list of the server to find matches. The initial state is then that, with unrecognised role-holders being flagged for removal by inserting their Discord ID instead of their twitch handle into the state (this is both unique and will never match a Twitch username).

## Comparing Msg and Role
msg is the more suitable design, since it gives a sequential consistency guarantee to the state, and Discord API rate limits are scoped to the channel/role, i.e. exactly the requests managed by a single instance of msg/role, so they can be paced precisely in series.

Similarly, msg is extensible to multiple channels because of its agent class and bindable functions, which could just as easily be applied to role.

I came up with the design for msg later, but left role as-is to show off different concurrency setups (threads vs promises) and structuring in Go.
