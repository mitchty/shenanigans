package filecache

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/google/uuid"
)

type CachedFile struct {
	Uri       string
	Sha256Sum string
	Remote    RemoteFile
}

type RemoteFile struct {
	Dest  string
	Mode  string
	Owner string
	Group string
}

// TODOmove appname to somewhere... else future mitch problem

// Hopefully this never changes, its chefs kiss for a name and nobody
// can stop me from using it I'm an adult allegedly.
const APPNAME = "shenanigans"

// Return the cacheDir for the app
func CacheDir() string {
	return path.Join(xdg.CacheHome, APPNAME)
}

// TmpDir in the same fs as ^^^
func TmpDir() string {
	// os.Getpid() should be enough to guarantee a unique run.
	//
	// If someone manages to get pid rollover and hit the same pid
	// they should immediately go to Vegas.
	tmpDir := path.Join(CacheDir(), "tmp", fmt.Sprintf("%d", os.Getpid()))
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() error {
		<-sigs
		err := os.Remove(tmpDir)
		return err
	}()
	return tmpDir
}

// If an os signal is caught, this should be called to clean up any
// tempdir setup.
func CleanupCache() error {
	err := os.RemoveAll(TmpDir())
	if err != nil {
		return err
	}
	return nil
}

// Downloads file to .cache/shenanigans/uuid and then validates the
// sha256sums match what was provided and then moves the file to its
// sha256sum in .config/shenanigans for further usage.
func CacheInput(input CachedFile) error {
	dlUuid := uuid.New().String()

	// sha := input.Sha256Sum
	// if input.Sha256Sum != "" {
	// 	sha = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	// }
	// destFile := path.Join(CacheDir(), "cache", sha)
	destFile := path.Join(CacheDir(), "cache", input.Sha256Sum)

	e, error := os.Stat(destFile)
	// We only download if we don't find a file already in
	// ~/.cache/sha25sum, if we have it there we assume its ok and
	// hasn't bitrotted.
	if !errors.Is(error, os.ErrNotExist) {
		if e.Size() != 0 {
			return nil
		}

		return nil
	}

	err := os.MkdirAll(TmpDir(), os.ModePerm)
	if err != nil {
		_ = fmt.Errorf(err.Error())
		return err
	}

	dlFile := path.Join(TmpDir(), dlUuid)

	out, err := os.Create(dlFile)
	defer out.Close()
	if err != nil {
		return err
	}

	fmt.Printf("dl: %s\n", input.Uri)
	resp, err := http.Get(input.Uri)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	dlSum, err := Sha256Sum(dlFile)
	if err != nil {
		return err
	}

	if dlSum == input.Sha256Sum {
		_, error := os.Stat(destFile)

		if errors.Is(error, os.ErrNotExist) {
			os.Rename(dlFile, destFile)
		}

		// fmt.Printf("reusing already cached file from uri %s expected sha256sum: %s found: %s\n", uri, shasum, dlSum)
	} else {
		// Don't leave turds around in ~/.cache/shenanigans/tmp
		os.Remove(dlFile)
		log.Fatal(fmt.Sprintf("file from uri %s expected sha256sum: %s found: %s\n", input.Uri, input.Sha256Sum, dlSum))
		return err
	}
	return nil
}

// Derp function to read a file and get a sha256sum
func Sha256Sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		_ = fmt.Errorf(err.Error())
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		_ = fmt.Errorf(err.Error())
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
