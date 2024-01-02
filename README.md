# Git repository version

`gv` A standalone tool to get version information from git repository

Install

```shell
go install github.com/yougg/gv@latest
```

Usage

```shell
# show help
gv -h

# only get version from git repo
gv -r /path/to/repo
cd /path/to/repo && gv

# get all version information from git repo
gv -a -r /path/to/repo
cd /path/to/repo && gv -a
```

Example

> `gv -r /path/to/gv`  
> main-20240102183907-759ac82df558

> `cd /path/to/gv; gv -a`  
> Version: main-20240102183907-759ac82df558  
> Tag:  
> Branch: main  
> CommitTime: 20240102183907  
> CommitID: 759ac82df558dbabbc1890c108bdff9ebd5a8c79

Ignore error log output

```shell
gv 2> /dev/null
```