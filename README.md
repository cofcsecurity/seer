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
├ parent: 0
├ user: root  euid: 0
├ exe deleted: false
└ md5: 7063c3930affe123baecd3b340f1ad2c
```

List processes and related sockets:
```
root@system:/# seer proc ls --socket 
┬─[1] /usr/bin/bash (/bin/bash) root 98s
├┬[11] /usr/bin/nc.traditional (nc-lp42) root 83s
│└─<0> tcp 0.0.0.0:42 <- 0.0.0.0:0 (LISTEN) i:467275
├┬[43] /usr/bin/nc.traditional (nc192.168.42.180) root 4s
│└─<1> tcp 172.17.0.2:59626 -> 192.168.42.1:80 (ESTABLISHED) i:466804
└─[44] /usr/local/bin/seer (seerprocls--socket) root 0s
```

Show a process tree:
```
root@system:/# seer proc tree 
┬[1] /usr/bin/bash /bin/bash
├┬[9] /usr/bin/screen SCREEN-Sx
│└┬[10] /usr/bin/dash /bin/sh
│ └┬[11] /usr/bin/bash bash
│  └─[13] /usr/bin/nc.traditional nc-lp42
└─[47] /usr/local/bin/seer seerproctree
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
