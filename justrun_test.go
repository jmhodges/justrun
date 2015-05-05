// These will all be very flaky because they are dependent on actual
// filesystems.
package main

import (
	"crypto/rand"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const waitForMsg = 2 * time.Second

// Slow in the success case
func TestNoWatchingCreation(t *testing.T) {
	fs := newFS(t)
	fs.Create("foobar")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs("foobar")}, []string{}, ch)
	defer cleanUp()
	fs.Create("baz")
	seeNothing(fs, ch, "creation of baz")
}

// Slow in the success case
func TestSimpleWatch(t *testing.T) {
	fs := newFS(t)
	fs.Create("foobar")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs("foobar")}, []string{fs.Abs("baz")}, ch)
	defer cleanUp()
	fs.ChangeContents("foobar")
	seeChangeContents(fs, ch, "foobar")
	fs.Create("baz")
	seeNothing(fs, ch, "creation of baz")
}

func TestSimpleDirWatch(t *testing.T) {
	fs := newFS(t)
	fs.MkdirAll("simpleDir1")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs("simpleDir1")}, []string{}, ch)
	defer cleanUp()

	fs.Create("simpleDir1/foobar")
	seeCreation(fs, ch, "simpleDir1/foobar")
}

func TestCurrentDirWorks(t *testing.T) {
	fs := newFS(t)
	fs.Create("foobar")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs(".")}, nil, ch)
	defer cleanUp()
	fs.ChangeContents("foobar")
	seeChangeContents(fs, ch, "foobar")
	fs.Create("baz")
	seeCreation(fs, ch, "baz")
}

// Slow in the success case
func TestIgnoredDir(t *testing.T) {
	fs := newFS(t)
	fs.Create("foobar")
	fs.MkdirAll("existdir/quuxdir")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs(".")},
		[]string{
			fs.Abs("bardir"),
			fs.Abs("existdir"),
		},
		ch)
	defer cleanUp()

	fs.MkdirAll("bardir")
	fs.Create("bardir/barfile")
	fs.RemoveAll("bardir")
	fs.Create("existdir/level1")
	fs.Create("existdir/quuxdir/level2")
	fs.ChangeContents("existdir/level1")
	fs.ChangeContents("existdir/quuxdir/level2")
	seeNothing(fs, ch, "some ignored operations")
}

// Slow in the success case
func TestIgnoredDirOverlap(t *testing.T) {
	fs := newFS(t)
	fs.Create("foobar")
	fs.MkdirAll("existdir/quuxdir")
	ch := make(chan time.Time, 10)
	// The existdir and existdir/quuxdir cases seem silly but can
	// happen accidentally in shell globbing. Better to be consistent
	// about it.
	cleanUp := watchTest(fs,
		[]string{
			fs.Abs("."),
			fs.Abs("bardir"),
			fs.Abs("existdir/quuxdir"),
		},
		[]string{
			fs.Abs("bardir"),
			fs.Abs("existdir/quuxdir"),
		},
		ch)
	defer cleanUp()

	fs.MkdirAll("bardir")
	fs.Create("bardir/barfile")
	fs.Create("existdir/level1")
	fs.Create("existdir/quuxdir/level2")
	fs.ChangeContents("existdir/level1")
	fs.ChangeContents("existdir/quuxdir/level2")

	seeNothing(fs, ch, "some ignored operations")
}

func TestNoSubdirRecursionWithoutGlobs(t *testing.T) {
	fs := newFS(t)
	fs.MkdirAll("some_dir")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs, []string{fs.Abs(".")}, nil, ch)
	defer cleanUp()

	fs.Create("some_dir/level1")
	seeNothing(fs, ch, "creation in level1")
}

func TestRenameFile(t *testing.T) {
	fs := newFS(t)

	fs.Create("foobar")
	ch := make(chan time.Time, 10)

	cleanUp := watchTest(fs, []string{fs.Abs("foobar")}, []string{}, ch)
	defer cleanUp()
	renameTest(fs, ch, "foobar", "baz")
	renameTest(fs, ch, "baz", "foobar")
}

func TestRenameDir(t *testing.T) {
	fs := newFS(t)

	fs.MkdirAll("foodir")
	fs.Create("foodir/foobar")
	ch := make(chan time.Time, 10)

	cleanUp := watchTest(fs, []string{fs.Abs("foodir/foobar")}, []string{}, ch)
	defer cleanUp()
	renameTest(fs, ch, "foodir/foobar", "baz")
	renameTest(fs, ch, "baz", "foodir/foobar")
	renameTest(fs, ch, "foodir", "bardir")
	// TODO(jmhodges): this requires a better rename system than
	// fsnotify provides or watching every dang directory in the
	// entiry absolute path.
	// renameTest(fs, ch, "bardir", "foodir")
}

func TestHiddenFilesHiddenByDefault(t *testing.T) {
	fs := newFS(t)
	fs.MkdirAll("hDir1")
	fs.Create("hDir1/.hidden")
	fs.MkdirAll("hDir2")
	fs.MkdirAll("hDir3")
	fs.Create("hDir3/.hiddenAndIgnored")
	ch := make(chan time.Time, 10)
	cleanUp := watchTest(fs,
		[]string{fs.Abs("hDir1/.hidden"), fs.Abs("hDir2")},
		[]string{fs.Abs("hDir3/.hiddenAndIgnored")},
		ch)
	defer cleanUp()

	fs.ChangeContents("hDir1/.hidden")
	seeChangeContents(fs, ch, "hDir1/.hidden")
	fs.Create("hDir2/.hidden")
	seeNothing(fs, ch, "no hidden file creation")
	fs.ChangeContents("hDir3/.hiddenAndIgnored")
	seeNothing(fs, ch, "no event for changes to hDir3/.hiddenAndIgnored")
}

func renameTest(fs *fileSystem, ch <-chan time.Time, oldpath, newpath string) {
	fs.Rename(oldpath, newpath)
	seeRename(fs, ch, oldpath, newpath)
}

func seeRename(fs *fileSystem, ch <-chan time.Time, oldpath, newpath string) {
	select {
	case <-ch:
		fs.t.Logf("successful catch of rename ('%s' -> '%s')", oldpath, newpath)
	case <-time.After(waitForMsg):
		fs.t.Errorf("did not see rename ('%s' -> '%s')", oldpath, newpath)
	}
}

func seeNothing(fs *fileSystem, ch <-chan time.Time, msg string) {
	select {
	case <-ch:
		fs.t.Errorf("should not have seen anything but saw: %s", msg)
	case <-time.After(waitForMsg):
		fs.t.Logf("successfully saw nothing for '%s'", msg)
	}
}

func seeCreation(fs *fileSystem, ch <-chan time.Time, path string) {
	select {
	case <-ch:
		fs.t.Logf("successful catch of creation of '%s'", path)

	case <-time.After(waitForMsg):
		fs.t.Errorf("did not see creation of '%s'", path)

	}
}

func seeChangeContents(fs *fileSystem, ch <-chan time.Time, path string) {
	select {
	case <-ch:
		fs.t.Logf("successful catch of first event (MODIFY|ATTRIB) for changing contents of '%s'", path)
	case <-time.After(waitForMsg):
		fs.t.Errorf("did not see content change of '%s'", path)
	}
	select {
	case <-ch:
		fs.t.Logf("successful catch of second event (MODIFY) for changing contents of '%s'", path)
	case <-time.After(waitForMsg):
		fs.t.Errorf("did not see content change of '%s'", path)
	}
}

func watchTest(fs *fileSystem, inputPaths, ignoredPaths []string, cmdCh chan<- time.Time) func() {
	w, err := watch(inputPaths, ignoredPaths, cmdCh)
	if err != nil {
		fs.t.Fatalf("unable to run watch: %#v", err)
		return func() {}
	}
	return func() {
		w.Close()
		fs.Close()
	}
}

func newFS(t *testing.T) *fileSystem {
	name, err := ioutil.TempDir("", "justrun_tests_")
	subdir := filepath.Join(name, "subdir_for_fds")
	os.MkdirAll(subdir, 0700)
	if err != nil {
		t.Fatalf("unable to create temporary directory")
	}
	return &fileSystem{name: subdir, t: t}
}

type fileSystem struct {
	name string
	t    *testing.T
}

func (fs *fileSystem) Create(path string) {
	fullName := filepath.Join(fs.name, path)
	// O_SYNC is important
	f, err := os.OpenFile(fullName, os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0666)
	if err != nil {
		fs.t.Fatalf("unable to create '%s' in '%s': %#v", path, fs.name, err)
	}
	f.Close()
}

func (fs *fileSystem) MkdirAll(path string) {
	fullName := filepath.Join(fs.name, path)
	err := os.MkdirAll(fullName, 0700)
	if err != nil {
		fs.t.Fatalf("unable to create directory '%s' in '%s': %#v", path, fs.name, err)
	}
}

func (fs *fileSystem) Rename(oldpath, newpath string) {
	err := os.Rename(filepath.Join(fs.name, oldpath), filepath.Join(fs.name, newpath))
	if err != nil {
		fs.t.Fatalf("unable to rename '%s' to '%s' in '%s'", oldpath, newpath, fs.name)
	}
}

func (fs *fileSystem) Abs(path string) string {
	return filepath.Join(fs.name, path)
}

func (fs *fileSystem) RemoveAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		fs.t.Fatalf("unable to delete '%s': %s", path, err)
	}
}

func (fs *fileSystem) ChangeContents(path string) {
	fullName := filepath.Join(fs.name, path)
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fs.t.Fatalf("unable to get random data to change the contents of '%s': %s", path, err)
	}

	f, err := os.OpenFile(fullName, os.O_WRONLY|os.O_SYNC|os.O_TRUNC, 0666)
	if err != nil {
		fs.t.Fatalf("unable to open '%s' to change its contents: %s", path, err)
	}
	defer f.Close()
	n, err := f.Write(b)
	if err != nil {
		fs.t.Fatalf("unable to write new contents to '%s': %s", path, err)
	}
	if n < len(b) {
		fs.t.Fatalf("unable to write all contents to '%s'", path)
	}
}

func (fs *fileSystem) Close() {
	err := os.RemoveAll(fs.name)
	if err != nil {
		fs.t.Fatalf("unable to delete directory '%s'", fs.name)
	}
}
