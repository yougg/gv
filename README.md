# Git repository version

`gv` A standalone tool to get version information from git repository

## Install

```shell
go install github.com/yougg/gv@latest
```

## Usage

```shell
# show help
gv -h

# only get version from git repo
gv -r /path/to/repo
cd /path/to/repo && gv

# get full version information from git repo
gv -a -r /path/to/repo
cd /path/to/repo && gv -a

# get version with branch name if no tag on HEAD
gv -a -b -r /path/to/repo
```

## Example

> `gv -r /path/to/gv`  
> v0.0.0-20240102183907-759ac82df558

> `gv -b -r /path/to/gv`  
> main-20240102183907-759ac82df558

> `cd /path/to/gv; gv -a`  
> Version: v0.0.0-20240102183907-759ac82df558  
> Tag:  
> Branch: main  
> CommitTime: 20240102183907  
> CommitID: 759ac82df558dbabbc1890c108bdff9ebd5a8c79

Ignore error log output

```shell
gv 2> /dev/null
```

## Use Case

add one source file `hello.go`

```go
package main

import "fmt"

var Version string

func main() {
	fmt.Println("Version:", Version)
}
```

commit and build the source file with `gv` version info

```shell
git init
git add hello.go
git commit -m 'initial commit'
go build -ldflags "-s -w -X main.Version=$(gv)" -o hello hello.go

./hello
# Version: v0.0.0-20240102234342-eab50ab71e12
gv -a
# Version: v0.0.0-20240102234342-eab50ab71e12
# Tag:
# Branch: main
# CommitTime: 20240102234342
# CommitID: eab50ab71e12b13b0030ecc05565dddc62f82af6
```

add tag then build and run again

```shell
git tag v0.0.1
go build -ldflags "-s -w -X main.Version=$(gv)" -o hello hello.go

./hello
# Version: v0.0.1
gv -a
# Version: v0.0.1
# Tag: v0.0.1
# Branch: main
# CommitTime: 20240102234342
# CommitID: eab50ab71e12b13b0030ecc05565dddc62f82af6
```