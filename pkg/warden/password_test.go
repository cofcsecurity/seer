package warden

import (
	"strings"
	"testing"
)

// Vectors from "Unix crypt using SHA-256 and SHA-512" by Ulrich Drepper.
func TestSHA512Crypt(t *testing.T) {
	cases := []struct {
		salt, password string
		rounds         int
		want           string
	}{
		{"saltstring", "Hello world!", 5000,
			"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"saltstringsaltstring", "Hello world!", 10000,
			"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v."},
		{"toolongsaltstring", "This is just a test", 5000,
			"$6$toolongsaltstrin$lQ8jolhgVRVhY4b5pZKaysCLi0QBxGoNeKQzQ3glMhwllF7oGDZxUhx1yxdYcz/e1JSbq3y6JMxxl8audkUEm0"},
	}
	for _, c := range cases {
		if got := sha512Crypt(c.password, c.salt, c.rounds); got != c.want {
			t.Errorf("sha512Crypt(%q, %q, %d)\n got %s\nwant %s", c.password, c.salt, c.rounds, got, c.want)
		}
	}
}

func TestHashPassword(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$6$rounds=100000$") || strings.Count(h, "$") != 4 {
		t.Fatalf("hash %q", h)
	}
	if err := CheckPasswordHash(h); err != nil {
		t.Fatal(err)
	}
	salt := strings.Split(h, "$")[3]
	if again := sha512Crypt("correct horse battery", salt, passwordRounds); again != h {
		t.Fatal("hash does not verify")
	}
}

func TestCheckPasswordHash(t *testing.T) {
	for _, bad := range []string{"", "hunter2", "$1$abc$def", "$6$salt", "$6$a b$c"} {
		if CheckPasswordHash(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, good := range []string{"$6$salt$hash", "$y$j9T$salt$hash", "$6$rounds=5000$salt$hash"} {
		if err := CheckPasswordHash(good); err != nil {
			t.Errorf("rejected %q: %v", good, err)
		}
	}
}
