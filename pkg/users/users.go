package users

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type User struct {
	// From /etc/passwd
	Username string
	Password Password
	Uid      int    // User id
	Gecos    string // Comma separated user details
	Home     string // Home directory
	Shell    string // Login shell

	// Groups that the user is a member of
	PrimaryGroup    Group
	SecondaryGroups []Group
}

func (u User) Expire() error {
	if !u.Password.IsExpired() {
		cmd := exec.Command("usermod", "-e", "1", u.Username)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to expire user: %s", err)
		}
	}
	return nil
}

func (u User) UnExpire() error {
	if u.Password.IsExpired() {
		cmd := exec.Command("usermod", "-e", "99999", u.Username)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to unexpire user: %s", err)
		}
	}
	return nil
}

func (u User) String() string {
	return fmt.Sprintf("%s (%d) %s %s %t\n",
		u.Username,
		u.Uid,
		u.Shell,
		u.Password.Password,
		u.Password.IsExpired(),
	)
}

func (u User) Describe() string {
	desc := "┌ %s (%d)\n"
	desc += "├ Home: %s\n"
	desc += "├ Shell: %s\n"
	desc += "├ Primary Group: %s\n"
	desc += "├ Secondary Groups: %s\n"
	desc += "├ Password: %s\n"
	desc += "└ Expired: %t\n"

	secondary_groups := make([]string, 0)
	for _, g := range u.SecondaryGroups {
		secondary_groups = append(secondary_groups, g.Name)
	}
	return fmt.Sprintf(
		desc,
		u.Username,
		u.Uid,
		u.Home,
		u.Shell,
		u.PrimaryGroup.Name,
		secondary_groups,
		u.Password.Password,
		u.Password.IsExpired())
}

func GetUsers() (map[string]User, error) {
	passwords, err := GetPasswords()
	if err != nil {
		log.Printf("Warning: %s\n", err)
		passwords = map[string]Password{}
	}
	groups, err := GetGroups()
	if err != nil {
		log.Printf("Warning: %s\n", err)
		groups = map[string]Group{}
	}
	groups_map := make(map[string][]Group)
	for g := range groups {
		for _, m := range groups[g].Members {
			groups_map[m] = append(groups_map[m], groups[g])
		}
	}

	passwd, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, errors.New("failed to read /etc/passwd")
	}
	defer passwd.Close()

	users := make(map[string]User)
	scanner := bufio.NewScanner(passwd)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 || line[0] == '#' {
			continue
		}
		user_data := strings.Split(line, ":")
		if len(user_data) < 7 {
			continue
		}

		user_id, err := strconv.Atoi(user_data[2])
		if err != nil {
			user_id = -1
		}
		user_gid, err := strconv.Atoi(user_data[3])
		if err != nil {
			user_gid = -1
		}
		var primary_group Group
		for _, g := range groups {
			if g.Id == user_gid {
				primary_group = g
				break
			}
		}

		user := User{
			Username:        user_data[0],
			Password:        passwords[user_data[0]],
			Uid:             user_id,
			Gecos:           user_data[4],
			Home:            user_data[5],
			Shell:           user_data[6],
			PrimaryGroup:    primary_group,
			SecondaryGroups: groups_map[user_data[0]],
		}

		users[user.Username] = user
	}

	return users, nil
}
