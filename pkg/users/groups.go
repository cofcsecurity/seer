package users

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type GPassword struct {
	// From /etc/gshadow
	Group    string   // The name of the group this password is for
	Password string   // Encrypted password
	Admins   []string // List of group admin usernames
	Members  []string // List of users who can access the group without the password
}

type Group struct {
	// From /etc/group
	Name     string
	Password GPassword // Group password (almost never used)
	Id       int       // Group id
	Members  []string  // List of users in the group
}

func (g Group) Describe() string {
	desc := "┌ %s (%d)\n"
	desc += "├ Password: %s\n"
	desc += "└ Members: %s\n"
	return fmt.Sprintf(
		desc,
		g.Name,
		g.Id,
		g.Password.Password,
		g.Members)
}

func GetGPasswords() (map[string]GPassword, error) {
	gshadow, err := os.Open("/etc/gshadow")
	if err != nil {
		return nil, errors.New("failed to read /etc/gshadow")
	}
	defer gshadow.Close()

	gpasswds := make(map[string]GPassword)
	scanner := bufio.NewScanner(gshadow)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 || line[0] == '#' {
			continue
		}
		gpasswd_data := strings.Split(line, ":")
		if len(gpasswd_data) < 4 {
			continue
		}

		admins := []string{}
		if len(gpasswd_data[2]) > 1 {
			admins = strings.Split(gpasswd_data[2], ",")
		}

		members := []string{}
		if len(gpasswd_data[3]) > 1 {
			members = strings.Split(gpasswd_data[3], ",")
		}

		gpasswd := GPassword{
			Group:    gpasswd_data[0],
			Password: gpasswd_data[1],
			Admins:   admins,
			Members:  members,
		}

		gpasswds[gpasswd.Group] = gpasswd
	}

	return gpasswds, nil
}

func GetGroups() (map[string]Group, error) {
	gpasswds, err := GetGPasswords()
	if err != nil {
		log.Printf("Warning: %s\n", err)
		gpasswds = make(map[string]GPassword)
	}

	group_db, err := os.Open("/etc/group")
	if err != nil {
		return nil, errors.New("failed to read /etc/group")
	}
	defer group_db.Close()

	groups := make(map[string]Group)
	scanner := bufio.NewScanner(group_db)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 || line[0] == '#' {
			continue
		}
		group_data := strings.Split(line, ":")
		if len(group_data) < 4 {
			continue
		}

		id, err := strconv.Atoi(group_data[2])
		if err != nil {
			id = -1
		}

		members := []string{}
		if len(group_data[3]) > 0 {
			members = strings.Split(group_data[3], ",")
		}

		group := Group{
			Name:     group_data[0],
			Password: gpasswds[group_data[0]],
			Id:       id,
			Members:  members,
		}

		groups[group.Name] = group
	}

	return groups, nil
}
