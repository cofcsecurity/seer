# seer

Extendable system enumeration and administration tool for linux

## Setup

### Installation

After cloning this repository run the following commands from the root level of the project:
```
go build -o seer
mv seer /usr/local/bin/
chmod +x /usr/local/bin
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

List processes
```
root@system:/# seer procs list
┌ [1] (bash)
├ state: S tty: 34816 session: 1 ppid: 0
├ uid: 0 euid: 0
└ link: /usr/bin/bash md5: 7063c3930affe123baecd3b340f1ad2c
┌ [10] (seer)
├ state: R tty: 34816 session: 1 ppid: 1
├ uid: 0 euid: 0
└ link: /usr/local/bin/seer md5: 6e1039cb500090fc441fff8dd08d1e7b
```

Expire any users whose name ends in "-contractor"
```
root@system:/# seer user expire -r "\-contractor$"
The following 2 user(s) will be modified:
  alice-contractor
  bob-contractor
Continue? (yes/no): yes
root@system:/# 
```
