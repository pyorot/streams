package main

import (
	"fmt"
	"time"

	dir "github.com/Pyorot/streams/dir"
	. "github.com/Pyorot/streams/utils"
)

// provides a task, role(), to call to process updates to roles

var roleID string                   // Discord ID of the role
var serverID string                 // Discord ID of the server the role belongs to
var roles = make(map[string]string) // map of managed users (Twitch username → Discord userID). inclusion = has role

// non-blocking http req to load all users and register to state via inverse look-up
func roleInit() chan (bool) {
	res := make(chan (bool), 1) // returned immediately; posted to when done
	go func() {                 // anonymous function in new thread; posts to res when done
		// create inverse dict to identify for each discord user if eir stream is still up
		inverseDir := dir.Inverse()
		// find every discord member with the role and register using dir
		next := ""     // ID of next user, used to chain sync calls (endpoint has 1000-result limit)
		userCount := 0 // will track total users detected
		for {
			users, err := discord.GuildMembers(serverID, next, 1000)
			ExitIfError(err)
			if len(users) == 0 { // found all users
				break
			} else { // process data and set "next" ahead of next call to see if there's more
				next = users[len(users)-1].User.ID
				userCount += len(users)
				for _, user := range users {
					for _, role := range user.Roles {
						if role == roleID { // if managed role is in user's roles
							twitchHandle, isInDir := inverseDir[user.User.ID]
							if isInDir {
								roles[twitchHandle] = user.User.ID
							} else { // if unknown user, trigger role-removal by registering under unique non-existent handle
								roles[user.User.ID] = user.User.ID
							}
							break
						}
					}
				}
			}
		}
		Log.Insta <- fmt.Sprintf("r | init [%d/%d] (%s)", len(roles), userCount, serverID)
		res <- true
	}()
	return res
}

// non-blocking parallelised call to all role additions/removals, handling return values
func role(new map[string]*stream) {
	// call external actions
	addsCh := make(map[string]chan (bool))    // list of chans to await additions
	removesCh := make(map[string]chan (bool)) // list of chans to await removals
	for user := range roles {                 // iterate thru old to pick removals
		_, isInNew := new[user]
		if !isInNew {
			Log.Insta <- "r | - " + user
			removesCh[user] = roleRemove(roles[user]) // async call; registers await chan
		}
	}
	for user := range new { // iterate thru new to pick additions
		_, isInOld := roles[user]
		userID := dir.Get(user) // look-up Twitch username in dir (skip user if not found)
		if !isInOld && userID != "" {
			Log.Insta <- "r | + " + user
			addsCh[user] = roleAdd(userID) // async call; registers await chan
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
			roles[user] = dir.Get(user)
		}
	}

	Log.Bkgd <- fmt.Sprintf("r | ok [%d]", len(roles))
}

// non-blocking http req to add role to user; returns channel to await success/failure
func roleAdd(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleAdd(serverID, userID, roleID)
		time.Sleep(1 * time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			Log.Insta <- fmt.Sprintf("x | r+ | %s : %s", userID, err)
		}
		res <- err == nil
	}()
	return res
}

// non-blocking http req to remove role from user; returns channel to await success/failure
func roleRemove(userID string) chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		err := discord.GuildMemberRoleRemove(serverID, userID, roleID)
		time.Sleep(1 * time.Second) // avoid 5 posts / 5s rate limit
		if err != nil {
			Log.Insta <- fmt.Sprintf("x | r- | %s : %s", userID, err)
		}
		res <- err == nil
	}()
	return res
}
