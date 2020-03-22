# Streams
This is a bot that **tracks who's streaming a particular game** on Twitch, and:
* posts + updates the stream into a Discord (messages) channel
* assigns the streamer's Discord account a special role while the stream is up

**Messages**  
The current design of the messages channel is that new streams are posted with a green embed, and when a stream goes offline, its post is edited to red and swapped with the oldest green post. Hence, active streams are at the front but all history is preserved. The old streams are ordered by end time, but also contain start time in the post itself.

**Roles**  
*Since Discord doesn't reveal a user's associated Twitch account to bots, the roles functionality requires manually-posted lists of Discord user IDs and associated Twitch usernames, marked "twicord", in a separate Discord channel.*

**Deployment**  
The bot is designed to run on [Heroku](https://www.heroku.com) PaaS (cloud deployment), able to resume its work after being restarted at any time, with config variables [stored outside Git](https://12factor.net/config). Deployment is done the standard way (i.e. create a Heroku Git remote, set config vars, then push/start/stop the bot).

**Scalability**  
With some further work to add class-like things to represent a message channel or a role, and a better config system, this could be scaled to handle many games, channels and roles, since it's fully concurrent.

# Files
* **main/main.go** – source for the main program + Twitch functionality
* **main/msg.go** – source for the Discord message channel functionality
* **main/main.go** – source for the Discord role functionality
* **go.mod** – build config (= Go version + versioned external dependencies)
* **Procfile** – command telling Heroku how to run the program

# Config
Config is loaded from a .env file in the same directory as the executable (built by running `go build main` to generate `main.exe`) if it's there (for local deployment). On Heroku, you'd input config via the `heroku config` command instead. This is interpreted as environment variables. The settings are:
* **TWITCH_KEY** – Twitch API key
* **DISCORD_TOKEN** – Discord API token
* **GAME_ID** – ID of the game to track (requires an API request to find out)
* **TWICORD_CHANNEL_ID** – ID of Discord channel for loading twicord directory
* **CHANNEL_ID** – ID of Discord streams channel
* **ROLE_ID** – ID of Discord streams role
* **ROLE_SERVER_ID** – ID of Discord server containing streams role

# Design
## Live
The bot runs a `main()` loop, which sync pulls streams data for the specified game, then distributes it to `msg()` and `role()` concurrently, sleeping for a min before going again.

`msg()` is a co-routine thread, which reads in the current streams of the game, then compares this to its cached view of current streams, issuing a list of add/edit/remove commands. These are then synchronously processed (retry until success), and when done, the co-routine is able to read in the next input.

`role()` is an async task, like a JS promise. It gets passed the current streams, compares them to its cached view, then issues in parallel all of its commands, awaiting confirmation before updating its state, then returning (if error, state isn't updated, so the command is dropped until next run).

*`msg` is the more suitable design, since it gives a sequential consistency guarantee to the state, and Discord API rate limits are scoped to the message channel, i.e. exactly the requests managed by this thread, so they can be paced precisely in series. I came up with this design later, but left `role` as-is to show off different concurrency setups in Go.*

## Persistence
Any changes to or recovery of the bot are done by restarting it, at any time. It recovers its state like this:

`msgInit()` looks at the last 50 messages in its thread and takes ownership of any representing active streams, reading info about a stream from its message into the state.

`roleInit()` finds every user in the Discord server who has the managed role, and flags it for removal by inserting into the state a key with an impossible name, value pointing to the user's ID. These roles are all removed, then reapplied in case the stream is still active.

`twicordInit()` just rebuilds the `twicord` lookup table.
