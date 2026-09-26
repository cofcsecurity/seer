# SSH command family

The `ssh` commands inspect live connections, server configuration, authorized
keys, and login history. They also provide two actions: ending one inbound
connection and removing one authorized key. The commands are registered in
[`cmd/ssh/ssh.go`](../cmd/ssh/ssh.go). Command files handle arguments, output,
and confirmation; [`pkg/ssh`](../pkg/ssh) reads system data and performs the
checks before an action.

This document describes the sources and checks used by the current code. See
the [README](../README.md#ssh-inspection-and-administration) for example output.

## Live connections: `list` and `describe`

[`pkg/ssh/sessions.go`](../pkg/ssh/sessions.go) builds a live connection from
an SSH process that owns an established TCP socket. It does not require a
terminal or a login record. The process scan works as follows:

1. Read numeric entries in `/proc`. For each process, read `/proc/PID/stat` for
   its parent and start time, `/proc/PID/status` for its UID, and `/proc/PID/exe`
   and `/proc/PID/cmdline` for its identity and display name.
2. Read `/proc/PID/ns/net` to group processes by network namespace. Read
   `/proc/PID/fd` symlinks to find the socket inodes they own.
3. Read `/proc/PID/net/tcp` and `tcp6` through a process in each visible
   namespace. Decode the IPv4 and IPv6 addresses and keep established sockets.
4. Join each socket to its owners by namespace and inode. An `sshd` or
   `sshd-session` executable makes it inbound; an `ssh` executable makes it
   outbound. Multiple processes holding one socket produce one displayed
   connection with an owner count.

The scan also looks for an `sshd` listening socket in the same namespace and
on the connection's local port. That sets `SSH listener matched`, which is
required by `ssh end`. The displayed login user can come from a descendant
process whose `SSH_CONNECTION` environment value matches the socket's client
and server addresses. The process UID is shown separately because an SSH
server process may run as root while its logged-in user is someone else.

[`pkg/ssh/records.go`](../pkg/ssh/records.go) adds information when available.
It asks `loginctl` for remote sessions and matches each session leader to an
SSH process by ancestry. It also runs `who -u`, which reads the system's
utmp/utmpx provider, and matches the reported PID by ancestry. These sources
can add a login name, login time, and authentication evidence. They do not
create live connections in the list: a command without a TTY may have no
login record, and an old record cannot prove a socket is still open.

`Authenticated` means one of the matching child environment or login records
was found. If none is visible, the connection remains in the list with
`auth unknown`. Reading other processes' file descriptors, environments, and
namespaces usually requires root. When some reads fail, the scan warns that
connections may be missing. Processes can also exit during a scan.

### Session IDs and the current connection

Seer makes each displayed ID from the process ID, `/proc/PID/stat` start ticks,
socket inode, and an FNV hash of the namespace and two socket addresses. SSH
and Linux do not assign this combined ID. A fresh ID helps the action code
notice when a process or socket has changed; it is not a permanent login ID.

The current connection is identified when Seer is a descendant of the inbound
SSH process or its own `SSH_CONNECTION` value matches the socket. The list
includes `[CURRENT SESSION]`. [`cmd/ssh/display.go`](../cmd/ssh/display.go)
colors it yellow only when output is an interactive terminal with color
available. The text marker remains in redirected or uncolored output.

### Ending a connection: `end`

[`cmd/ssh/end.go`](../cmd/ssh/end.go) lists connections again, displays the
selected connection, and asks for confirmation. If it is the current
connection, or `SSH_CONNECTION` is set but Seer cannot identify which
connection is current, it prints a red warning when color is available and
requires two separate `yes` answers. `--yes` does not skip those answers.
Other connections need one confirmation unless `--yes` is used.

[`pkg/ssh/end.go`](../pkg/ssh/end.go) then lists connections again and checks
the full ID, PID, start ticks, and socket inode. It requires an inbound SSH
process, a matching SSH listener, and exactly one process owner for the
socket. It also checks the current-connection rule again. If any check fails,
no signal is sent.

On Linux amd64 and arm64, [`pkg/ssh/signal_linux.go`](../pkg/ssh/signal_linux.go)
opens a pidfd for the selected process, checks its start time, namespace,
socket inode, and executable again, and rescans the connection and listener.
Only then does it send SIGTERM through the pidfd. A pidfd pins the process
identity; the repeated socket checks narrow the time between inspection and
action. They cannot make changing socket state fully atomic. Other platforms
return an unsupported error. A shared socket may prevent ending a connection;
the code refuses to guess which process to signal.

## Server configuration: `config list` and `config check`

[`pkg/ssh/config.go`](../pkg/ssh/config.go) reads `/etc/ssh/sshd_config` by
default, or the file selected with `--file`.

- `config list` prints active rule lines with file paths and line numbers. It
  follows `Include` globs recursively and reports unreadable included files.
  This is a view of written rules, including `Match` lines, not a calculation
  of which rules apply to one connection.
- `config check` runs the installed `sshd -t -f FILE` for syntax validation,
  then `sshd -T -f FILE` for effective settings. It passes supplied `--user`,
  `--addr`, `--host`, `--laddr`, and `--lport` values through `sshd -T -C` so
  `Match` rules can be evaluated for that context. It prints selected warning
  and review findings, followed by the effective settings in sorted order.

The findings identify settings worth reviewing, such as direct root login or
password authentication. They are prompts for an operator; the right policy
depends on the host. `config check` needs a usable local `sshd` executable.
For a relative `Include` pattern, the rule listing currently resolves it
under `/etc/ssh`, even when `--file` selects another directory.

## Authorized keys: `keys list`, `describe`, and `remove`

[`pkg/ssh/keys.go`](../pkg/ssh/keys.go) obtains accounts from `getent passwd`
when available, with `/etc/passwd` as a fallback. For each selected account,
it asks `sshd -T -C user=...` for `AuthorizedKeysFile` and expands `%h`, `%u`,
`%U`, and `%%` in those paths. `--addr` supplies a client address for relevant
`Match` rules. The code reads the resulting files, parses active key lines,
and hashes the decoded public key data to produce SHA256 fingerprints. The
list shows the account, fingerprint, key type, file and line, and comment;
`describe` also shows key options.

If the effective configuration cannot be read, listing falls back to
`.ssh/authorized_keys` and `.ssh/authorized_keys2` in each home directory and
prints a warning. Removal requires the effective `AuthorizedKeysFile` setting;
it does not use the fallback paths. Keys supplied by `AuthorizedKeysCommand`
or another external source are outside this file inventory. A missing client
address may also leave address-specific `Match` rules unevaluated for the
connection of interest.

[`cmd/ssh/keys.go`](../cmd/ssh/keys.go) selects one fingerprint. When a
fingerprint appears more than once, `--user`, `--path`, and `--line` narrow the
entry. Removing a key for the current SSH account, or removing a key when the
current SSH login cannot be identified, requires `--allow-current-access`.
After confirmation, `RemoveKey` verifies the effective key path and selected
line again. It makes a backup in the same directory, writes a replacement
file, preserves its permissions, ownership, and extended attributes where
supported, and compares the original content and file identity before
renaming the replacement over it. It checks that the selected entry is gone.
The backup path is printed even if a later replacement step fails. As with
any file edited by another process at the same time, a change after the final
comparison remains possible.

## Login history: `history` and `history --failed`

[`pkg/ssh/history.go`](../pkg/ssh/history.go) reads available sources
separately. It queries `journalctl` for `sshd` and `sshd-session` process names
or syslog identifiers, scans SSH daemon lines in `/var/log/auth.log`,
`/var/log/secure`, `/var/log/messages`, and `/var/log/syslog`, and runs `last`
for system login records. With `--failed`, it filters journal and text lines
for common failed authentication messages and runs `lastb` for bad login
records. `--limit` applies to each source, not to the combined output.

`last` and `lastb` obtain wtmp and btmp records through the system tools;
Seer does not read those binary files directly. Their locations and
availability vary by distro. Those records are system-wide, so one record
does not by itself prove SSH was used. Journal and text logs may report the
same event twice, and the failed-message filter does not cover every possible
SSH daemon message. History never establishes whether a connection is live.

## Maintaining this family

Keep argument handling and presentation in `cmd/ssh`, and system inspection
and action checks in `pkg/ssh`. When adding another source of login context,
attach it to an already observed transport and add its name to `Sources`;
do not make it sufficient evidence for a live connection. When changing an
action, keep the checks at the point of action as well as in the command that
displays the target. The package tests use synthetic `/proc` data and file
fixtures for these rules. `go test ./...` and `go vet ./...` check the Go code;
ending a connection still needs testing on a Linux host with an expendable
SSH session.
