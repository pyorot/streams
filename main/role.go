package main

import (
	"fmt"
	"time"
)

var roles = make(map[string]string) // map of managed users (Twitch username → Discord userID)

// non-blocking http req to load all users, then flag them for role removal (so we start afresh)
func roleInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		next := ""     // ID of next user, used to chain calls (endpoint has 1000-result limit)
		userCount := 0 // will track total users detected
		for {
			users, err := discord.GuildMembers(env["ROLE_SERVER_ID"], next, 1000)
			exitIfError(err)
			if len(users) == 0 { // found all users
				break
			} else { // process data and set "next" to see if there's more
				next = users[len(users)-1].User.ID
				userCount += len(users)
				for _, user := range users {
					for _, role := range user.Roles {
						if role == env["ROLE_ID"] { // if managed role is in user's roles
							roles[user.User.ID] = user.User.ID // register user under Discord user ID *instead of Twitch username*
							break                              // so that next incoming data comparison will def trigger a role-removal
						}
					}
				}
			}
		}
		fmt.Printf("r | init [%d/%d]\n", len(roles), userCount)
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
			time.Sleep(2 * time.Second)               // avoid 5 posts / 5s rate limit
			fmt.Printf("r | - %s\n", user)
		}
	}
	for user := range new { // iterate thru new to pick additions
		_, isInOld := roles[user]
		userID, isReg := twicord[user] // look-up Twitch username in twicord (will ignore user if not found)
		if !isInOld && isReg {
			addsCh[user] = roleAdd(userID) // async call; registers await chan
			time.Sleep(2 * time.Second)    // avoid 5 posts / 5s rate limit
			fmt.Printf("r | + %s\n", user)
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

	fmt.Printf("r | ok [%d]\n", len(roles))
}

// non-blocking http req to add role to user; returns success
func roleAdd(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleAdd(env["ROLE_SERVER_ID"], userID, env["ROLE_ID"])
		if err != nil {
			fmt.Printf("x | r+ | %s – %s\n", userID, err)
		}
		res <- err == nil
	}()
	return res
}

// non-blocking http req to remove role from user; returns success
func roleRemove(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleRemove(env["ROLE_SERVER_ID"], userID, env["ROLE_ID"])
		if err != nil {
			fmt.Printf("x | r- | %s – %s\n", userID, err)
		}
		res <- err == nil
	}()
	return res
}
