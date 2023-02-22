# seer

Extendable system enumeration and administration tool for linux

## Setup

### Installation

After cloning this repository run the following commands from the root level of the project:
```
go build -o seer
mv seer /usr/local/bin/
chmod +x /usr/local/bin/seer
```

### Completion

To generate an autocompletion script for your terminal use the `seer completion` command.

The following commands can be used to configure bash autocompletion:
```
apt update && apt install bash-completion -y
mkdir /etc/bash_completion.d/
seer completion bash > /etc/bash_completion.d/seer
echo "source /etc/bash_completion" >> /etc/bash.bashrc
```

For other shells see `seer completion -h`

## Usage

Seer contains a number of subcommands for tasks ranging from querying system processes to expiring any user on the system that matches a regex pattern.

For a complete list of subcommands and options use `seer -h` and `seer [subcommand] -h`.

### Examples

List and describe processes
```
root@system:/# seer proc list
[1] /usr/bin/bash (/bin/bash) root 42s
[54] /usr/local/bin/seer (seerproclist) root 0s
root@system:/# seer proc describe 1
┌[1] /usr/bin/bash
├ cmdline: /bin/bash
├ state: S age: 65s
├ parent: 0 (sched)
├ user: root  euid: 0
├ exe deleted: false
└ md5: 7063c3930affe123baecd3b340f1ad2c
```

Describe the user `alice`
```
root@system:/# seer user describe alice
┌ alice (1000)
├ Home: /home/alice
├ Shell: /bin/sh
├ Primary Group: alice
├ Secondary Groups: [sudo]
├ Password: !
└ Expired: false
```

Expire any users whose name ends in `-contractor`
```
root@system:/# seer users expire -r "\-contractor$"
The following 2 user(s) will be modified:
  mallory-contractor
  bob-contractor
Continue? (yes/no): yes
Modified 2 user(s).
```
