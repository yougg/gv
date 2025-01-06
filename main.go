//go:generate go build -trimpath -ldflags "-s -w" $GOFILE
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

var (
	all   bool
	showb bool
	repo  string
)

func init() {
	flag.BoolVar(&all, `a`, false, "show all version information")
	flag.BoolVar(&showb, `b`, false, "show branch name instead of tag")
	flag.StringVar(&repo, `r`, ``, "git repository path")
	flag.Usage = func() {
		fmt.Println("Usage: gv")
		flag.PrintDefaults()
		fmt.Println("Example:")
		fmt.Println("\tgv -r /path/to/repo/")
		fmt.Println("\tgv -a -r /path/to/repo/")
		fmt.Println("\tcd /path/to/repo/ && gv")
		fmt.Println("\tcd /path/to/repo/ && gv -a")
	}
	flag.Parse()
}

// read .git for version information
func main() {
	var gitRoot string
	if len(repo) > 0 {
		gitRoot = repo
		if gitRoot != `` && filepath.Base(gitRoot) != `.git` {
			gitRoot = filepath.Join(gitRoot, `.git`)
		}
	} else {
		gitRoot = getGitRoot()
	}
	if gitRoot == `` || filepath.Base(gitRoot) != `.git` {
		slog.Error("can not find .git dir for repo", `path`, gitRoot)
		return
	}
	Version(gitRoot)
}

func getGitRoot() (gitRoot string) {
	wd, err := os.Getwd()
	if err != nil {
		slog.Error("get current working dir", `err`, err)
		return ``
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		slog.Error("get wd absolute path", `err`, err)
		return ``
	}
	for range [3]struct{}{} { // recursive find '.git' dir from './' or '../' or '../../'
		_ = filepath.Walk(wd, func(path string, info fs.FileInfo, err error) error {
			if !info.IsDir() {
				return nil
			}
			if filepath.Base(path) == `.git` {
				gitRoot = path
				return filepath.SkipAll
			}
			return nil
		})
		if gitRoot != `` {
			break
		}
		wd = filepath.Dir(wd)
	}
	return
}

// Version get version at HEAD
func Version(gitRoot string) {
	tag, err := findTag(gitRoot)
	if err != nil {
		slog.Error(`find tag`, `err`, err)
		return
	}
	var version string
	if tag != `` {
		version = tag
		fmt.Print(tag)
		if !all {
			return
		}
	}

	line, err := getLastLineWithSeek(gitRoot)
	if err != nil {
		slog.Error("get last line", `err`, err)
		return
	}
	fields := strings.Split(line, ` `)
	if l := len(fields); l < 6 {
		slog.Error("get invalid commit record", `line`, line)
		return
	}
	commitID, commitTime := fields[1], fields[4]
	if len(commitID) < 40 || len(commitTime) < 10 {
		slog.Error("get invalid commit ID/time", `commitID`, commitID, `commitTime`, commitTime)
		return
	}
	branch, err := matchBranch(gitRoot, commitID)
	if err != nil {
		slog.Error("match branch", `err`, err)
		return
	}
	if branch == `` {
		branch, err = findBranch(gitRoot)
		if err != nil {
			slog.Error("find branch", `err`, err)
			return
		}
	}

	var ref string
	tag, err = nearliestTag(gitRoot, branch)
	if err == nil && tag != `` {
		ref = tag
	} else if showb {
		ref = branch
	} else {
		ref = `v0.0.0`
	}

	timestamp, err := strconv.ParseInt(commitTime, 10, 64)
	if err != nil {
		slog.Error("parse commit time", `err`, err)
		return
	}
	date := time.Unix(timestamp, 0).Format(`20060102150405`)
	if version == `` {
		version = fmt.Sprintf("%s-%s-%s", ref, date, commitID[:12])
	}

	if all {
		fmt.Println(`Version: ` + version)
		fmt.Println(`Tag: ` + tag)
		fmt.Println(`Branch: ` + branch)
		fmt.Println(`CommitTime: ` + date)
		fmt.Println(`CommitID: ` + commitID)
	} else {
		fmt.Print(version)
	}
}

func getLastLineWithSeek(gitRoot string) (string, error) {
	file, err := os.Open(filepath.Join(gitRoot, `logs/HEAD`))
	if err != nil {
		return "", fmt.Errorf("os open file: %w", err)
	}
	defer file.Close()

	var line []byte
	stat, _ := file.Stat()
	fileSize := stat.Size()
	// read file byte by byte with reverse order until the head of last line
	for cursor := int64(-1); cursor > -fileSize; cursor-- {
		_, err = file.Seek(cursor, io.SeekEnd)
		if err != nil {
			return "", fmt.Errorf("file seek to %d: %w", cursor, err)
		}

		char := make([]byte, 1)
		count, err := file.Read(char)
		if err != nil {
			return "", fmt.Errorf("read 1 byte from file: %w", err)
		}
		if count != 1 {
			continue
		}
		if cursor != -1 && (char[0] == 10 || char[0] == 13) {
			break
		}

		line = append(line, char...)
	}

	// reverse line content
	for l, r := 0, len(line)-1; l < r; l, r = l+1, r-1 {
		line[l], line[r] = line[r], line[l]
	}

	return string(line), nil
}

// findTag get tag at HEAD if it exists
func findTag(gitRoot string) (tag string, err error) {
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		err = fmt.Errorf("git open repository path %s: %w", filepath.Dir(gitRoot), err)
		return
	}
	h, err := repo.Head()
	if err != nil {
		err = fmt.Errorf("get repository head: %w", err)
		return
	}
	tags, err := repo.Tags()
	if err != nil {
		err = fmt.Errorf("get repository tags: %w", err)
		return
	}
	err = tags.ForEach(func(reference *plumbing.Reference) error {
		if reference.Hash() == h.Hash() {
			tag = reference.Name().Short()

			return storer.ErrStop
		}
		return nil
	})
	return

	// fallback to run git command
	//	1: git tag --points-at HEAD
	//	2: git pack-refs; awk -F 'tags/' /$(git rev-parse HEAD)/'{print $2}' .git/packed-refs
	//err = os.Chdir(filepath.Dir(gitRoot))
	//if err != nil {
	//	slog.Error("change dir", `err`, err)
	//	return
	//}
	//cmd := exec.Command(`sh`, `-c`, `git tag --points-at HEAD 2> /dev/null | sort -V | tail -1`)
	//output, err := cmd.Output()
	//if err != nil {
	//	slog.Error("git cmd output", `err`, err)
	//	return
	//}
	//tag = string(output)
}

// nearliestTag find nearliest tag from given branch
func nearliestTag(gitRoot, branch string) (tag string, err error) {
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		err = fmt.Errorf("git open repository path %s: %w", filepath.Dir(gitRoot), err)
		return
	}
	h, err := repo.Head()
	if err != nil {
		err = fmt.Errorf("get repository head: %w", err)
		return
	}
	branches, err := repo.Branches()
	if err != nil {
		err = fmt.Errorf("get branches: %w", err)
		return
	}
	tags, err := repo.Tags()
	if err != nil {
		err = fmt.Errorf("get repository tags: %w", err)
		return
	}
	err = branches.ForEach(func(reference *plumbing.Reference) error {
		commits, err := repo.Log(&git.LogOptions{From: reference.Hash()})
		if err != nil {
			return err
		}
		if err = commits.ForEach(func(commit *object.Commit) error {
			if commit.Hash == h.Hash() {
				branch = reference.Name().Short()
				return storer.ErrStop
			}
			return nil
		}); err != nil || branch == `` {
			return err
		}
		var tagRefs []*plumbing.Reference
		if err = tags.ForEach(func(reference *plumbing.Reference) error {
			tagRefs = append(tagRefs, reference)
			return nil
		}); err != nil || len(tagRefs) == 0 {
			return err
		}
		slices.Reverse(tagRefs)
		for _, ref := range tagRefs {
			if err = commits.ForEach(func(commit *object.Commit) error {
				if ref.Hash() == commit.Hash {
					tag = ref.Name().Short()
					return storer.ErrStop
				}
				return nil
			}); err == nil && tag != `` {
				break
			}
		}
		if tag != `` {
			return storer.ErrStop
		}
		return nil
	})
	return
}

// matchBranch match branch by HEAD commit ID
func matchBranch(gitRoot, commitID string) (branch string, err error) {
	var content []byte
	err = filepath.Walk(filepath.Join(gitRoot, `refs/heads`), func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		content, err = os.ReadFile(path)
		if err != nil {
			err = fmt.Errorf("read file %s: %w", path, err)
			return err
		}
		if bytes.Contains(content, []byte(commitID)) {
			branch = filepath.Base(path)
		}
		return nil
	})
	if branch == `` {
		content, err = os.ReadFile(filepath.Join(gitRoot, `HEAD`))
		if err != nil {
			err = fmt.Errorf("read file: %w", err)
			return "", err
		}

		fields := bytes.Split(bytes.TrimSpace(content), []byte{'/'})
		if l := len(fields); l >= 3 {
			branch = string(fields[l-1])
		}
	}
	return
}

// findBranch get branch where the HEAD belongs to.
func findBranch(gitRoot string) (branch string, err error) {
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		err = fmt.Errorf("git open repository path %s: %w", filepath.Dir(gitRoot), err)
		return
	}
	h, err := repo.Head()
	if err != nil {
		err = fmt.Errorf("get repository head: %w", err)
		return
	}
	branches, err := repo.Branches()
	if err != nil {
		err = fmt.Errorf("get branches: %w", err)
		return
	}
	err = branches.ForEach(func(reference *plumbing.Reference) error {
		commits, err := repo.Log(&git.LogOptions{From: reference.Hash()})
		if err != nil {
			return err
		}
		err = commits.ForEach(func(commit *object.Commit) error {
			if commit.Hash == h.Hash() {
				branch = reference.Name().Short()
				return storer.ErrStop
			}
			return nil
		})
		if branch != `` {
			return storer.ErrStop
		}
		return err
	})
	return
}
