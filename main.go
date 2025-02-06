//go:generate go build -trimpath -ldflags "-s -w" $GOFILE
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

var (
	all   bool
	showb bool
	repo  string

	ErrTagNotFound = errors.New(`tag not found`)

	verReg = regexp.MustCompile(`(v?)(\d+)\.(\d+)\.(\d+)`)
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
	} else if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], `-`) {
		gitRoot = getGitRoot(os.Args[1])
	} else {
		gitRoot = getGitRoot()
	}
	if gitRoot == `` || filepath.Base(gitRoot) != `.git` {
		slog.Error("can not find .git dir for repo", `path`, gitRoot)
		return
	}
	Version(gitRoot)
}

func getGitRoot(dir ...string) (gitRoot string) {
	var wd string
	var err error
	if len(dir) > 0 {
		wd = dir[0]
	} else {
		wd, err = os.Getwd()
		if err != nil {
			slog.Error("get current working dir", `err`, err)
			return ``
		}
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		slog.Error("get wd absolute path", `err`, err)
		return ``
	}
	for range 3 { // recursive find '.git' dir from './' or '../' or '../../'
		if err = filepath.Walk(wd, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return nil
			}
			if filepath.Base(path) == `.git` {
				gitRoot = path
				return filepath.SkipAll
			}
			return nil
		}); err != nil {
			slog.Error("walk git repo dir fail", `err`, err)
			return
		}
		if gitRoot != `` {
			break
		}
		wd = filepath.Dir(wd)
	}
	return
}

// Version get version at HEAD
func Version(gitRoot string) {
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		slog.Error("git open repository", `path`, filepath.Dir(gitRoot), `err`, err)
		return
	}
	head, err := repo.Head()
	if err != nil {
		slog.Error("get repository head", `err`, err)
		return
	}

	tag, err := findTag(repo, head.Hash())
	if err != nil && !errors.Is(err, ErrTagNotFound) {
		slog.Error(`find tag`, `err`, err)
		return
	}
	var version string
	if tag != `` {
		version = extractVersion(tag)
		fmt.Print(tag)
		if !all {
			return
		}
	}

	branch, err := findBranch(repo, head)
	if err != nil {
		slog.Error("find branch", `err`, err)
		return
	}

	var ref string
	tag, err = nearliestTag(repo, branch)
	if err == nil && tag != `` {
		ref = extractVersion(tag, true)
	} else if showb {
		ref = branch
	} else {
		ref = `v0.0.0`
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		slog.Error("get commit object", `err`, err)
		return
	}
	date := commit.Committer.When.Format(`20060102150405`)
	if version == `` {
		version = fmt.Sprintf("%s-%s-%s", ref, date, head.Hash().String()[:12])
	}

	if all {
		fmt.Println(`Version: ` + version)
		fmt.Println(`Tag: ` + tag)
		fmt.Println(`Branch: ` + branch)
		fmt.Println(`CommitTime: ` + date)
		fmt.Println(`CommitID:`, head.Hash())
	} else {
		fmt.Print(version)
	}
}

// findTag get tag at HEAD if it exists
func findTag(repo *git.Repository, head plumbing.Hash) (tag string, err error) {
	tags, err := repo.Tags()
	if err != nil {
		err = fmt.Errorf("get repository tags: %w", err)
		return
	}
	var tagNames []string
	if err = tags.ForEach(func(reference *plumbing.Reference) error {
		if reference.Hash() == head {
			tagNames = append(tagNames, reference.Name().Short())
			return storer.ErrStop
		}
		return nil
	}); err != nil {
		err = fmt.Errorf("get repository tags: %w", err)
		return
	}
	if len(tagNames) == 0 {
		err = ErrTagNotFound
		return
	}
	slices.Reverse(tagNames)
	tag = tagNames[0]
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

// nearliestTag find the nearliest tag from given branch
func nearliestTag(repo *git.Repository, branch string) (tag string, err error) {
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
	tagRefs := make(map[plumbing.Hash][]string)
	if err = tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsTag() {
			names, ok := tagRefs[ref.Hash()]
			if ok && names != nil {
				names = append(names, ref.Name().Short())
			} else {
				names = []string{ref.Name().Short()}
			}
			slices.Reverse(names)
			tagRefs[ref.Hash()] = names
		}
		return nil
	}); err != nil || len(tagRefs) == 0 {
		return
	}
	var tagNames []string
	err = branches.ForEach(func(reference *plumbing.Reference) error {
		if reference.Name().IsBranch() && reference.Name().Short() != branch {
			return nil // continue
		}
		commits, err := repo.Log(&git.LogOptions{From: reference.Hash()})
		if err != nil {
			return err
		}
		if err = commits.ForEach(func(commit *object.Commit) error {
			if names, ok := tagRefs[commit.Hash]; ok && len(names) > 0 {
				tagNames = append(tagNames, names...)
				return storer.ErrStop
			}
			return nil
		}); err != nil {
			return nil
		}
		if len(tagNames) > 0 {
			tag = tagNames[0]
		}
		if tag != `` {
			return storer.ErrStop
		}
		return nil
	})
	return
}

// findBranch get branch where the HEAD belongs to.
func findBranch(repo *git.Repository, head *plumbing.Reference) (branch string, err error) {
	if head.Name().IsBranch() {
		return head.Name().Short(), nil
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
			if commit.Hash == head.Hash() {
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

func extractVersion(tag string, add ...bool) string {
	match := verReg.FindStringSubmatch(tag)
	if len(match) == 0 {
		return tag
	}

	// increment patch version number
	patch, err := strconv.Atoi(match[4])
	if err != nil {
		return tag
	}
	if len(add) > 0 && add[0] {
		patch++
	}

	// 构造新的版本号
	version := `v` + match[2] + `.` + match[3] + `.` + strconv.Itoa(patch)
	return version
}
