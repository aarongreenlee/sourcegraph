// Package filter offers an http.FileSystem wrapper with the ability to ignore files.
package filter

import (
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"time"
)

// Func is a filtering function which is provided two arguments,
// its '/'-separated rooted absolute path (i.e., it always begins with "/"),
// and the os.FileInfo of the considered file.
//
// For example, if the considered file is named "a" and it's inside a directory "dir",
// then the value of path will be "/dir/a".
type Func func(path string, fi os.FileInfo) bool

// New creates a filesystem that contains everything in source, except files for which
// ignore returns true.
func New(source http.FileSystem, ignore Func) http.FileSystem {
	return &filterFS{source: source, ignore: ignore}
}

type filterFS struct {
	source http.FileSystem
	ignore Func // Skip files that ignore returns true for.
}

// clean turns a potentially relative path into an absolute one.
//
// This is needed to normalize path parameter for ignore func.
func (fs *filterFS) clean(path string) string {
	return pathpkg.Clean("/" + path)
}

func (fs *filterFS) Open(path string) (http.File, error) {
	f, err := fs.source.Open(path)
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if fs.ignore(fs.clean(path), fi) {
		// Skip.
		f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}

	if !fi.IsDir() {
		return f, nil
	}
	defer f.Close()

	fis, err := f.Readdir(0)
	if err != nil {
		return nil, err
	}

	var entries []os.FileInfo
	for _, fi := range fis {
		if fs.ignore(fs.clean(pathpkg.Join(path, fi.Name())), fi) {
			// Skip.
			continue
		}
		entries = append(entries, fi)
	}

	return &dir{
		name:    fi.Name(),
		entries: entries,
		modTime: fi.ModTime(),
	}, nil
}

// dir is an opened dir instance.
type dir struct {
	name    string
	modTime time.Time
	entries []os.FileInfo
	pos     int // Position within entries for Seek and Readdir.
}

func (d *dir) Read([]byte) (int, error) {
	return 0, fmt.Errorf("cannot Read from directory %s", d.name)
}
func (d *dir) Close() error               { return nil }
func (d *dir) Stat() (os.FileInfo, error) { return d, nil }

func (d *dir) Name() string       { return d.name }
func (d *dir) Size() int64        { return 0 }
func (d *dir) Mode() os.FileMode  { return 0755 | os.ModeDir }
func (d *dir) ModTime() time.Time { return d.modTime }
func (d *dir) IsDir() bool        { return true }
func (d *dir) Sys() interface{}   { return nil }

func (d *dir) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == os.SEEK_SET {
		d.pos = 0
		return 0, nil
	}
	return 0, fmt.Errorf("unsupported Seek in directory %s", d.name)
}

func (d *dir) Readdir(count int) ([]os.FileInfo, error) {
	if d.pos >= len(d.entries) && count > 0 {
		return nil, io.EOF
	}
	if count <= 0 || count > len(d.entries)-d.pos {
		count = len(d.entries) - d.pos
	}
	e := d.entries[d.pos : d.pos+count]
	d.pos += count
	return e, nil
}
