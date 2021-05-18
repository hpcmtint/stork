package storkutil

import (
	"io/ioutil"
	"log"
	"os"
	"path"
)

// Struct that holds information about sandbox.
type Sandbox struct {
	BasePath string
}

// Create a new sandbox. The sandbox is located in a temporary
// directory.
func NewSandbox() *Sandbox {
	dir, err := ioutil.TempDir("", "stork_ut_*")
	if err != nil {
		log.Fatal(err)
	}
	sb := &Sandbox{
		BasePath: dir,
	}

	return sb
}

// Close sandbox and remove all its contents.
func (sb *Sandbox) Close() {
	os.RemoveAll(sb.BasePath)
}

// Create parent directory in sandbox and all parent directories,
// create indicated file in this parent directory, and return a full
// path to this file.
func (sb *Sandbox) Join(name string) string {
	// build full path
	fpath := path.Join(sb.BasePath, name)

	// ensure directory
	dir := path.Dir(fpath)
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		log.Fatal(err)
	}

	// create file in the filesystem
	file, err := os.Create(fpath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	return fpath
}

// Create indicated directory in sandbox and all parent directories
// and return a full path.
func (sb *Sandbox) JoinDir(name string) string {
	// build full path
	fpath := path.Join(sb.BasePath, name)

	// ensure directory
	err := os.MkdirAll(fpath, 0777)
	if err != nil {
		log.Fatal(err)
	}

	return fpath
}
