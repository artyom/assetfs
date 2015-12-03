// +build ignore

package assetfs

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

func AssetDir(name string) http.FileSystem { return _assetFilesystems[name] }

// _assetfs implements http.FileSystem interface
type _assetfs struct {
	data  [][]byte        // depends on number of files
	meta  []_itemMetadata // depends on number of files+dirs, files going first
	names map[string]int  // key is index of meta
}

func (fs *_assetfs) Open(name string) (http.File, error) {
	if fs == nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: name,
			Err:  os.ErrNotExist,
		}
	}
	name = path.Clean("/" + name)
	i, ok := fs.names[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	fi := fs.meta[i]
	af := &_assetFile{
		fi: fi,
		fs: fs,
	}
	if !fi.isDir {
		af.rd = bytes.NewReader(fs.data[i])
	}
	return af, nil
}

// _itemMetadata implements os.FileInfo interface
type _itemMetadata struct {
	name     string
	size     int64
	mode     os.FileMode
	mtime    int64
	isDir    bool
	children []int // indexes of items in directories
}

func (m _itemMetadata) Name() string       { return m.name }
func (m _itemMetadata) Size() int64        { return m.size }
func (m _itemMetadata) Mode() os.FileMode  { return m.mode }
func (m _itemMetadata) ModTime() time.Time { return time.Unix(0, m.mtime) }
func (m _itemMetadata) IsDir() bool        { return m.isDir }
func (m _itemMetadata) Sys() interface{}   { return nil }

// _assetFile implements http.File interface
type _assetFile struct {
	rd   *bytes.Reader
	fi   _itemMetadata
	fs   *_assetfs
	read int // how many entries read already by Readdir
}

func (af *_assetFile) Close() error               { return nil } // TODO guard against usage after close?
func (af *_assetFile) Stat() (os.FileInfo, error) { return af.fi, nil }
func (af *_assetFile) Readdir(count int) ([]os.FileInfo, error) {
	if !af.fi.isDir {
		return nil, os.ErrInvalid
	}
	if count <= 0 {
		if af.read >= len(af.fi.children) {
			return nil, nil
		}
		out := make([]os.FileInfo, len(af.fi.children[af.read:]))
		for i, idx := range af.fi.children[af.read:] {
			out[i] = af.fs.meta[idx]
		}
		af.read += len(out)
		return out, nil
	}
	if af.read >= len(af.fi.children) {
		return nil, io.EOF
	}
	out := make([]os.FileInfo, 0, count)
	for i, idx := range af.fi.children[af.read:] {
		if i == count {
			break
		}
		out = append(out, af.fs.meta[idx])
	}
	af.read += len(out)
	return out, nil
}
func (af *_assetFile) Seek(offset int64, whence int) (int64, error) {
	if af.fi.isDir {
		return 0, _errIsDirectory
	}
	return af.rd.Seek(offset, whence)
}
func (af *_assetFile) Read(p []byte) (int, error) {
	if af.fi.isDir {
		return 0, _errIsDirectory
	}
	return af.rd.Read(p)
}

var _errIsDirectory = errors.New("is directory")

var _assetFilesystems = map[string]*_assetfs{
	"static": &_assetfs{
		data: [][]byte{
			[]byte("abc"),
			[]byte("cde"),
		},
		meta: []_itemMetadata{
			{name: "red", size: 3, mode: os.FileMode(0644), mtime: 0, isDir: false},
			{name: "green", size: 3, mode: os.FileMode(0644), mtime: 0, isDir: false},
			{name: "static", mode: os.FileMode(0755), isDir: true, children: []int{0, 1}},
		},
		names: map[string]int{
			"/red":   0,
			"/green": 1,
			"/":      2,
		},
	},
}
