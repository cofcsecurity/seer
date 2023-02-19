package users

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Password struct {
	// From /etc/shadow, usually requires root access
	Username          string
	Password          string // Encrypted password, "*", or "!"
	Last_change       int    // Date when the password was last changed, in days since epoch
	Min_age           int    // Number of days before the password can be changed, typically 0
	Max_age           int    // Number of days after a reset when the password expires, typically 99999
	Warn_period       int    // Number of days before password expiration to start warning the user
	Inactivity_period int    // Number of days after password expiration when the account is diabled, typically blank
	Expiration_date   int    // Date when the password expires, in days since epoch
}

func (p Password) IsExpired() bool {
	if p.Expiration_date != -1 {
		expires := time.Unix(int64(86400)*int64(p.Expiration_date), 0)
		if time.Now().After(expires) {
			return true
		}
	}
	return false
}

func GetPasswords() (map[string]Password, error) {
	shadow, err := os.Open("/etc/shadow")
	if err != nil {
		return nil, errors.New("failed to read /etc/shadow")
	}
	defer shadow.Close()

	passwords := make(map[string]Password)
	scanner := bufio.NewScanner(shadow)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 || line[0] == '#' {
			continue
		}
		password_data := strings.Split(line, ":")
		if len(password_data) < 8 {
			continue
		}

		last_change, err := strconv.Atoi(password_data[2])
		if err != nil {
			last_change = -1
		}

		min_age, err := strconv.Atoi(password_data[3])
		if err != nil {
			min_age = -1
		}
		max_age, err := strconv.Atoi(password_data[4])
		if err != nil {
			max_age = -1
		}

		warn_period, err := strconv.Atoi(password_data[5])
		if err != nil {
			warn_period = -1
		}

		inactivity_period, err := strconv.Atoi(password_data[6])
		if err != nil {
			inactivity_period = -1
		}

		expiration, err := strconv.Atoi(password_data[7])
		if err != nil {
			expiration = -1
		}

		password := Password{
			Username:          password_data[0],
			Password:          password_data[1],
			Last_change:       last_change,
			Min_age:           min_age,
			Max_age:           max_age,
			Warn_period:       warn_period,
			Inactivity_period: inactivity_period,
			Expiration_date:   expiration,
		}

		passwords[password.Username] = password
	}

	return passwords, nil
}
