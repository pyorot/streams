package main

import (
	"fmt"
	"time"
)

var roles = make(map[string]string)

func roleInit() chan (bool) {
	res := make(chan (bool), 1)
	go func() {
		next := ""
		userCount := 0
		for {
			users, err := discord.GuildMembers(env["ROLE_SERVER_ID"], next, 300)
			exitIfError(err)
			if len(users) == 0 {
				break
			} else {
				next = users[len(users)-1].User.ID
				userCount += len(users)
				for _, user := range users {
					for _, role := range user.Roles {
						if role == env["ROLE_ID"] {
							roles[user.User.ID] = user.User.ID
							break
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

func role(new map[string]*stream) {
	addsCh := make(map[string]chan (bool))
	removesCh := make(map[string]chan (bool))

	for user := range roles {
		_, isInNew := new[user]
		if !isInNew { // remove
			removesCh[user] = roleRemove(roles[user])
			time.Sleep(2 * time.Second)
			fmt.Printf("r | - %s\n", user)
		}
	}
	for user := range new {
		_, isInOld := roles[user]
		userID, isReg := twicord[user]
		if !isInOld && isReg { // add
			addsCh[user] = roleAdd(userID)
			time.Sleep(2 * time.Second)
			fmt.Printf("r | + %s\n", user)
		}
	}

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
