package warden

import (
	"bufio"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// cryptAlphabet is the base-64 alphabet crypt(3) uses for salts and hashes.
const cryptAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// passwordRounds slows offline guessing. Warden only accepts the hash on its
// command line, where local users can briefly see it, so the default 5000
// rounds is too cheap. Verifying 100000 rounds at login takes tens of ms.
const passwordRounds = 100000

// MinPasswordLength is the shortest team password Seer accepts.
const MinPasswordLength = 12

// HashPassword returns a SHA-512 crypt(3) hash ($6$) of plain with a random
// salt, in the form PAM and shadow accept.
func HashPassword(plain string) (string, error) {
	salt := make([]byte, 16)
	for i := range salt {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(cryptAlphabet))))
		if err != nil {
			return "", err
		}
		salt[i] = cryptAlphabet[n.Int64()]
	}
	return sha512Crypt(plain, string(salt), passwordRounds), nil
}

// CheckPasswordHash accepts the crypt(3) hash formats Linux shadow uses, so a
// pasted --password-hash is checked before any clone or download.
func CheckPasswordHash(h string) error {
	h = strings.TrimSpace(h)
	for _, prefix := range []string{"$6$", "$5$", "$y$", "$2b$", "$2y$"} {
		if strings.HasPrefix(h, prefix) && strings.Count(h, "$") >= 3 && !strings.ContainsAny(h, " \t\n:") {
			return nil
		}
	}
	return errors.New("--password-hash is not a crypt hash; expected one starting with $6$ (make one with: openssl passwd -6), or use --password to be prompted")
}

// sha512Crypt implements SHA-512 crypt as specified by Ulrich Drepper
// ("Unix crypt using SHA-256 and SHA-512"), which glibc uses for $6$.
func sha512Crypt(password, salt string, rounds int) string {
	if len(salt) > 16 {
		salt = salt[:16]
	}
	p, s := []byte(password), []byte(salt)

	b := sha512.New()
	b.Write(p)
	b.Write(s)
	b.Write(p)
	sumB := b.Sum(nil)

	a := sha512.New()
	a.Write(p)
	a.Write(s)
	i := len(p)
	for ; i > 64; i -= 64 {
		a.Write(sumB)
	}
	a.Write(sumB[:i])
	for i = len(p); i > 0; i >>= 1 {
		if i&1 != 0 {
			a.Write(sumB)
		} else {
			a.Write(p)
		}
	}
	sumA := a.Sum(nil)

	dp := sha512.New()
	for range p {
		dp.Write(p)
	}
	pSeq := repeatTo(dp.Sum(nil), len(p))

	ds := sha512.New()
	for n := 0; n < 16+int(sumA[0]); n++ {
		ds.Write(s)
	}
	sSeq := repeatTo(ds.Sum(nil), len(s))

	c := sumA
	for r := 0; r < rounds; r++ {
		h := sha512.New()
		if r&1 != 0 {
			h.Write(pSeq)
		} else {
			h.Write(c)
		}
		if r%3 != 0 {
			h.Write(sSeq)
		}
		if r%7 != 0 {
			h.Write(pSeq)
		}
		if r&1 != 0 {
			h.Write(c)
		} else {
			h.Write(pSeq)
		}
		c = h.Sum(nil)
	}

	var out strings.Builder
	out.WriteString("$6$")
	if rounds != 5000 {
		out.WriteString("rounds=" + strconv.Itoa(rounds) + "$")
	}
	out.WriteString(salt)
	out.WriteByte('$')
	order := [][3]int{
		{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4}, {47, 5, 26}, {6, 27, 48},
		{28, 49, 7}, {50, 8, 29}, {9, 30, 51}, {31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13},
		{56, 14, 35}, {15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19}, {62, 20, 41},
	}
	for _, o := range order {
		encode24(&out, c[o[0]], c[o[1]], c[o[2]], 4)
	}
	encode24(&out, 0, 0, c[63], 2)
	return out.String()
}

func repeatTo(block []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, block[:min(len(block), n-len(out))]...)
	}
	return out
}

func encode24(out *strings.Builder, b2, b1, b0 byte, n int) {
	w := uint(b2)<<16 | uint(b1)<<8 | uint(b0)
	for ; n > 0; n-- {
		out.WriteByte(cryptAlphabet[w&0x3f])
		w >>= 6
	}
}

// PromptPassword asks for the team password twice on the terminal with echo
// off, and checks that the entries match and are long enough.
func PromptPassword() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", errors.New("--password needs a terminal to prompt on; use --password-hash with a hash from: openssl passwd -6")
	}
	defer tty.Close()
	stty := func(arg string) error {
		cmd := exec.Command("stty", arg)
		cmd.Stdin = tty
		return cmd.Run()
	}
	if err := stty("-echo"); err != nil {
		return "", fmt.Errorf("cannot turn off terminal echo for the password prompt: %w", err)
	}
	defer stty("echo")

	in := bufio.NewReader(tty)
	read := func(prompt string) (string, error) {
		fmt.Fprint(tty, prompt)
		line, err := in.ReadString('\n')
		fmt.Fprintln(tty)
		return strings.TrimRight(line, "\r\n"), err
	}
	first, err := read("Team password: ")
	if err != nil {
		return "", err
	}
	if len(first) < MinPasswordLength {
		return "", fmt.Errorf("the team password must be at least %d characters; red team will try to guess it", MinPasswordLength)
	}
	second, err := read("Team password again: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("the two password entries differ; run the command again")
	}
	return first, nil
}
