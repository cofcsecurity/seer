package ssh

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type PrincipalFile struct {
	Path       string
	Principals []string
}

type HostKeySource struct {
	Path        string
	Fingerprint string
	Mode        os.FileMode
}

type AuthSourceReport struct {
	User                        string
	AuthorizedKeyFiles          []string
	AuthorizedKeysCommand       string
	AuthorizedKeysCommandUser   string
	TrustedUserCAKeys           string
	CAFingerprints              []string
	PrincipalFiles              []PrincipalFile
	AuthorizedPrincipalsCommand string
	HostKeys                    []HostKeySource
	StrictModes                 string
	Warnings                    []string
}

func AuthenticationSources(username, configPath, clientAddr string) (AuthSourceReport, error) {
	if username == "" {
		return AuthSourceReport{}, fmt.Errorf("--user is required to inspect authentication sources")
	}
	all, err := accounts()
	if err != nil {
		return AuthSourceReport{}, err
	}
	var account *Account
	for i := range all {
		if all[i].Name == username {
			account = &all[i]
			break
		}
	}
	if account == nil {
		return AuthSourceReport{}, fmt.Errorf("user %q not found", username)
	}
	values, err := EffectiveConfigValues(configPath, map[string]string{"user": username, "addr": clientAddr})
	if err != nil {
		return AuthSourceReport{}, err
	}
	return authSourcesFromValues(*account, values)
}

func authSourcesFromValues(account Account, values map[string][]string) (AuthSourceReport, error) {
	get := func(key string) string {
		entries := values[key]
		if len(entries) == 0 {
			return ""
		}
		return entries[len(entries)-1]
	}
	report := AuthSourceReport{
		User: account.Name, AuthorizedKeysCommand: get("authorizedkeyscommand"),
		AuthorizedKeysCommandUser:   get("authorizedkeyscommanduser"),
		TrustedUserCAKeys:           get("trustedusercakeys"),
		AuthorizedPrincipalsCommand: get("authorizedprincipalscommand"),
		StrictModes:                 get("strictmodes"),
	}
	keyPatterns, err := configArguments(get("authorizedkeysfile"))
	if err != nil {
		return report, err
	}
	report.AuthorizedKeyFiles, err = expandKeyPaths(keyPatterns, account)
	if err != nil {
		return report, err
	}
	principalPatterns, err := configArguments(get("authorizedprincipalsfile"))
	if err != nil {
		return report, err
	}
	principalPaths, err := expandKeyPaths(principalPatterns, account)
	if err != nil {
		return report, err
	}
	for _, path := range principalPaths {
		file := PrincipalFile{Path: path}
		data, err := os.ReadFile(path)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read principals file %s: %v", path, err))
		} else {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" && !strings.HasPrefix(line, "#") {
					file.Principals = append(file.Principals, line)
				}
			}
			if err := scanner.Err(); err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("could not finish principals file %s: %v", path, err))
			}
		}
		report.PrincipalFiles = append(report.PrincipalFiles, file)
	}
	if report.TrustedUserCAKeys != "" && report.TrustedUserCAKeys != "none" {
		if _, err := os.Stat(report.TrustedUserCAKeys); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not stat trusted user CAs %s: %v", report.TrustedUserCAKeys, err))
		}
		keys, err := keysInFile(report.TrustedUserCAKeys, account)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read trusted user CAs %s: %v", report.TrustedUserCAKeys, err))
		}
		for _, key := range keys {
			report.CAFingerprints = append(report.CAFingerprints, "SHA256:"+key.Fingerprint)
		}
	}
	for _, path := range values["hostkey"] {
		if path == "none" {
			continue
		}
		host := HostKeySource{Path: path}
		if info, err := os.Stat(path); err == nil {
			host.Mode = info.Mode().Perm()
		} else {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not stat host key %s: %v", path, err))
		}
		if public, err := keysInFile(path+".pub", account); err == nil && len(public) > 0 {
			host.Fingerprint = "SHA256:" + public[0].Fingerprint
		} else if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read host public key %s.pub: %v", path, err))
		} else {
			report.Warnings = append(report.Warnings, fmt.Sprintf("no readable host public key in %s.pub", path))
		}
		report.HostKeys = append(report.HostKeys, host)
	}
	return report, nil
}

func (r AuthSourceReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Authentication sources for %s:\n", r.User)
	for _, path := range r.AuthorizedKeyFiles {
		fmt.Fprintf(&b, "AuthorizedKeysFile: %s\n", path)
	}
	if len(r.AuthorizedKeyFiles) == 0 {
		b.WriteString("AuthorizedKeysFile: none or no matching files\n")
	}
	if r.AuthorizedKeysCommand != "" && r.AuthorizedKeysCommand != "none" {
		fmt.Fprintf(&b, "AuthorizedKeysCommand: %s (user: %s; external keys not enumerated)\n", r.AuthorizedKeysCommand, r.AuthorizedKeysCommandUser)
	}
	if r.TrustedUserCAKeys != "" && r.TrustedUserCAKeys != "none" {
		fmt.Fprintf(&b, "TrustedUserCAKeys: %s\n", r.TrustedUserCAKeys)
		for _, fingerprint := range r.CAFingerprints {
			fmt.Fprintf(&b, "  CA: %s\n", fingerprint)
		}
	}
	for _, file := range r.PrincipalFiles {
		fmt.Fprintf(&b, "AuthorizedPrincipalsFile: %s\n", file.Path)
		for _, principal := range file.Principals {
			fmt.Fprintf(&b, "  Principal: %s\n", principal)
		}
	}
	if r.AuthorizedPrincipalsCommand != "" && r.AuthorizedPrincipalsCommand != "none" {
		fmt.Fprintf(&b, "AuthorizedPrincipalsCommand: %s (external principals not enumerated)\n", r.AuthorizedPrincipalsCommand)
	}
	for _, host := range r.HostKeys {
		fmt.Fprintf(&b, "HostKey: %s mode:%04o", host.Path, host.Mode.Perm())
		if host.Fingerprint != "" {
			fmt.Fprintf(&b, " %s", host.Fingerprint)
		}
		b.WriteByte('\n')
	}
	if r.StrictModes != "" {
		fmt.Fprintf(&b, "StrictModes: %s\n", r.StrictModes)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", warning)
	}
	return b.String()
}
