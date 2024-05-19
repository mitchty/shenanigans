package cache

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/google/uuid"
)

type State struct {
	// Base path of where things get stored at runtime
	// ~/.cache/shenanigans/UUID/...
	base  string
	suuid string

	tmpdir      string
	artifactdir string
	instancedir string
	cachedir    string

	// Artifact files
	artifacts map[string]string
}

// Lets us dynamically setup an artifact dir based off subpath
// returns that path as a string
// func (s State) RegisterArtifactDir(path string) (dir string, e error) {
// }

// Functions to get the state
func (s State) Tmp() string {
	return s.tmpdir
}

func (s State) Artifact() string {
	return s.artifactdir
}

func (s State) Instance() string {
	return s.instancedir
}

func (s State) Cache() string {
	return s.cachedir
}

func (s State) Base() string {
	return s.base
}

func (s State) Uuid() string {
	return s.suuid
}

func (s State) Format(f fmt.State, r rune) {
	out := fmt.Sprintf("state: base=%s tmp=%s artifact=%s instance=%s cache=%s uuid=%s", s.Base(), s.Tmp(), s.Artifact(), s.Instance(), s.Cache(), s.Uuid())
	f.Write([]byte(out))
}

// Idea is we register all artifacts in a call that makes the prefix
// dir for them and record the full path in the artifact map for
// future call site usage as a key.
func (s State) RegisterArtifact(dir string) (string, error) {
	a := path.Join(s.Artifact(), dir)
	d := filepath.Dir(a)
	err := os.MkdirAll(d, os.ModePerm)
	if err != nil {
		return "", err
	}
	s.artifacts[dir] = a
	return a, nil
}

func NewState(dir string, suuid string) (*State, error) {
	s := &State{
		artifacts: make(map[string]string),
	}

	var dirs []string
	if suuid == "" {
		s.suuid = uuid.New().String()
	} else {
		s.suuid = suuid
	}

	// For testing let this all
	if dir != "" {
		s.base = dir
	} else {
		s.base = path.Join(xdg.CacheHome, "shenanigans", s.suuid)
	}

	dirs = append(dirs, s.base)

	s.tmpdir = path.Join(s.base, "tmp")
	s.artifactdir = path.Join(s.base, "artifacts")
	s.instancedir = path.Join(s.base, "instance")
	s.cachedir = path.Join(path.Join(xdg.CacheHome, "shenanigans"), "cache")

	dirs = append(dirs, s.tmpdir)
	dirs = append(dirs, s.artifactdir)
	dirs = append(dirs, s.cachedir)
	dirs = append(dirs, s.instancedir)
	dirs = append(dirs, s.tmpdir)

	for _, d := range dirs {
		err := os.MkdirAll(d, os.ModePerm)
		if err != nil {
			return s, err
		}
	}

	return s, nil
}
