package main

import (
	"fmt"
	"time"

	log "github.com/Pyorot/streams/log"
)

var roleID string
var roleServerID string

var roles = make(map[string]string) // map of managed users (Twitch username → Discord userID)

// non-blocking http req to load all users, then flag them for role removal (so we start afresh)
func roleInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		// create dict to identify per-discord-user if eir twitch stream is still up
		reverseTwicord := make(map[string]string, len(twicord))
		for k, v := range twicord {
			reverseTwicord[v] = k
		}
		// find every discord member with the role and register using twicord
		next := ""     // ID of next user, used to chain calls (endpoint has 1000-result limit)
		userCount := 0 // will track total users detected
		for {
			users, err := discord.GuildMembers(roleServerID, next, 1000)
			exitIfError(err)
			if len(users) == 0 { // found all users
				break
			} else { // process data and set "next" to see if there's more
				next = users[len(users)-1].User.ID
				userCount += len(users)
				for _, user := range users {
					for _, role := range user.Roles {
						if role == roleID { // if managed role is in user's roles
							twitchHandle, isInTwicord := reverseTwicord[user.User.ID]
							if isInTwicord {
								roles[twitchHandle] = user.User.ID
							} else { // if unknown user, trigger role-removal by registering under non-existent handle
								roles[user.User.ID] = user.User.ID
							}
							break
						}
					}
				}
			}
		}
		log.Insta <- fmt.Sprintf("r | init [%d/%d]", len(roles), userCount)
		res <- true
	}()
	return res
}

// non-blocking parallelised call to all role additions/removals, handling return values
func role(new map[string]*stream) {
	// perform external actions
	addsCh := make(map[string]chan (bool))    // list of chans to await additions
	removesCh := make(map[string]chan (bool)) // list of chans to await removals
	for user := range roles {                 // iterate thru old to pick removals
		_, isInNew := new[user]
		if !isInNew {
			removesCh[user] = roleRemove(roles[user]) // async call; registers await chan
			time.Sleep(1 * time.Second)               // avoid 5 posts / 5s rate limit
			log.Insta <- "r | - " + user
		}
	}
	for user := range new { // iterate thru new to pick additions
		_, isInOld := roles[user]
		userID, isReg := twicord[user] // look-up Twitch username in twicord (will ignore user if not found)
		if !isInOld && isReg {
			addsCh[user] = roleAdd(userID) // async call; registers await chan
			time.Sleep(1 * time.Second)    // avoid 5 posts / 5s rate limit
			log.Insta <- "r | + " + user
		}
	}

	// await results; hence update internal state (blocks on each await chan in series)
	for user, ch := range removesCh {
		if <-ch {
			delete(roles, user)
		}
	}
	for user, ch := range addsCh {
		if <-ch {
			roles[user] = twicord[user]
		}
	}

	log.Bkgd <- fmt.Sprintf("r | ok [%d]", len(roles))
}

// non-blocking http req to add role to user; returns success
func roleAdd(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleAdd(roleServerID, userID, roleID)
		if err != nil {
			log.Insta <- fmt.Sprintf("x | r+ | %s : %s", userID, err)
		}
		res <- err == nil
	}()
	return res
}

// non-blocking http req to remove role from user; returns success
func roleRemove(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleRemove(roleServerID, userID, roleID)
		if err != nil {
			log.Insta <- fmt.Sprintf("x | r- | %s : %s", userID, err)
		}
		res <- err == nil
	}()
	return res
}
