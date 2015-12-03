// Command assetfs generates http.FileSystem implementation compiling assets
// inside go binary as byte slices
//
// Can be used with go:generate, see example subdirectory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/artyom/autoflags"
)

const maxFileSize = 10 << 20

func main() {
	params := struct {
		FullOutput string `flag:"out,path to write generated content"`
		DevOutput  string `flag:"dev,path to write development stub"`
		FullTag    string `flag:"tag,build tag to use for main generated file"`
		DevTag     string `flag:"devtag,build tag to assign to development stub"`
		Package    string `flag:"name,package name"`
	}{
		DevTag:  "dev",
		Package: os.Getenv("GOPACKAGE"),
	}
	if gofile, ok := os.LookupEnv("GOFILE"); ok {
		gofile = strings.TrimSuffix(gofile, ".go")
		params.FullOutput = gofile + "_assetfs.go"
		params.DevOutput = gofile + "_assetfs-dev.go"
	}
	autoflags.Define(&params)
	flag.Parse()
	if params.Package == "" {
		log.Fatal("invalid package name")
	}
	if params.FullOutput == "" {
		log.Fatal("invalid output")
	}
	if params.FullOutput == params.DevOutput {
		log.Fatal("normal and dev output cannot be the same")
	}
	if params.DevOutput != "" && params.FullTag == params.DevTag {
		log.Fatal("normal and dev output should use different tags")
	}
	if len(flag.Args()) == 0 {
		log.Fatal("no asset directories provided")
	}
	if err := generateMain(params.FullOutput, params.Package, params.FullTag, flag.Args()); err != nil {
		log.Fatal(err)
	}
	if params.DevOutput != "" {
		if err := generateStub(params.DevOutput, params.Package, params.DevTag); err != nil {
			log.Fatal(err)
		}
	}
}

func generateStub(filename, pkg, tag string) error {
	if filename == "" {
		return errors.New("empty filename")
	}
	outfile, err := ioutil.TempFile("", "assetfs-stub-tmp.")
	if err != nil {
		return err
	}
	defer os.Remove(outfile.Name())
	defer outfile.Close()

	writer := ErrWriter(outfile)
	writeHeader(writer, pkg, tag)
	writer.Write([]byte(stub))
	if err := writer.Err(); err != nil {
		return err
	}
	if err := outfile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(outfile.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(outfile.Name(), filename)
}

func generateMain(filename, pkg, tag string, dirs []string) error {
	if filename == "" {
		return errors.New("empty filename")
	}
	outfile, err := ioutil.TempFile("", "assetfs-main-tmp.")
	if err != nil {
		return err
	}
	defer os.Remove(outfile.Name())
	defer outfile.Close()

	writer := ErrWriter(outfile)
	writeHeader(writer, pkg, tag)
	writer.Write([]byte(head))
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if err := writeSection(writer, dir); err != nil {
			return err
		}
	}
	writeTail(writer)
	if err := writer.Err(); err != nil {
		return err
	}
	if err := outfile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(outfile.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(outfile.Name(), filename)
}

func writeSection(wr io.Writer, dir string) error {
	fmt.Fprintf(wr, "\t%#q: &_assetfs{\n\t\tdata: [][]byte{\n", dir)
	tr := &tree{
		wr:      wr,
		root:    dir,
		names:   make(map[string]int),
		dirData: make(map[string]*dirInfo),
	}
	if err := filepath.Walk(tr.root, tr.walkFunc); err != nil {
		return err
	}
	tr.writeMetadata()
	return nil
}

type tree struct {
	wr        io.Writer
	root      string
	filesMeta []os.FileInfo
	dirMeta   []os.FileInfo
	names     map[string]int
	dirData   map[string]*dirInfo
}

type dirInfo struct {
	files   []int
	subdirs []int
}

func rootedName(name, root string) string {
	name = strings.TrimPrefix(name, root)
	if name == "" {
		return "/"
	}
	return name
}

func (tr *tree) writeMetadata() {
	info := append(tr.filesMeta, tr.dirMeta...)
	dirIdxShift := len(tr.filesMeta)
	indexes := make(map[string]int, len(tr.names))
	for name, i := range tr.names {
		normName := rootedName(name, tr.root)
		indexes[normName] = i
		if _, ok := tr.dirData[name]; ok {
			indexes[normName] += dirIdxShift
		}
	}
	subdir := make(map[string][]int, len(tr.dirData))
	for name, d := range tr.dirData {
		subIndexes := make([]int, 0, len(d.files)+len(d.subdirs))
		subIndexes = append(subIndexes, d.files...)
		for _, v := range d.subdirs {
			subIndexes = append(subIndexes, v+dirIdxShift)
		}
		subdir[rootedName(name, tr.root)] = subIndexes
	}
	idx2name := make(map[int]string, len(indexes))
	for k, v := range indexes {
		idx2name[v] = k
	}

	fmt.Fprintf(tr.wr, "\t\t},\n\t\tmeta: []_itemMetadata{\n")

	for i, fi := range info {
		fmt.Fprintf(tr.wr, "\t\t\t{name: %#q, mode: %#o, mtime: %d, ",
			fi.Name(), fi.Mode(), fi.ModTime().UnixNano())
		if !fi.IsDir() {
			fmt.Fprintf(tr.wr, "size: %d", fi.Size())
		} else {
			fmt.Fprint(tr.wr, "isDir: true")
			children := subdir[idx2name[i]]
			if len(children) > 0 {
				fmt.Fprintf(tr.wr, ", children: %#v", children)
			}
		}
		fmt.Fprint(tr.wr, "},\n")
	}
	fmt.Fprint(tr.wr, "\t\t},\n\t\tnames: map[string]int{\n")
	// deterministic order
	for i := 0; i < len(indexes); i++ {
		fmt.Fprintf(tr.wr, "\t\t\t%q: %d,\n", idx2name[i], i)
	}
	fmt.Fprint(tr.wr, "\t\t},\n\t},\n")
}
func writeTail(wr io.Writer) { wr.Write([]byte("}\n")) }

func writeHeader(wr io.Writer, pkg, tag string) {
	wr.Write([]byte("// autogenerated package, do not edit\n\n"))
	if tag != "" {
		fmt.Fprintf(wr, "// +build %s\n\n", tag)
	}
	fmt.Fprintf(wr, "package %s\n", pkg)
}

func (tr *tree) walkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.IsDir() {
		idx := len(tr.dirMeta)
		tr.dirMeta = append(tr.dirMeta, info)
		tr.names[path] = idx
		tr.dirData[path] = &dirInfo{}
		if pi, ok := tr.dirData[filepath.Dir(path)]; ok {
			pi.subdirs = append(pi.subdirs, idx)
		}
	} else {
		if info.Size() > maxFileSize {
			return fmt.Errorf("file %q size exceeds max allowed size", path)
		}
		idx := len(tr.filesMeta)
		tr.filesMeta = append(tr.filesMeta, info)
		tr.names[path] = idx
		if pi, ok := tr.dirData[filepath.Dir(path)]; ok {
			pi.files = append(pi.files, idx)
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(tr.wr, "\t\t\t[]byte(%#q),\n", data)
	}
	return nil
}

func ErrWriter(w io.Writer) *errWriter { return &errWriter{Writer: w} }

type errWriter struct {
	io.Writer
	err error // first error encountered
}

// Err returns first error writer encountered
func (ew *errWriter) Err() error { return ew.err }

// Write writes to underlying io.Writer, if previous write end up with non-nil
// error, subsequent calls would return this error and no writes would be done
// on underlying io.Writer
func (ew *errWriter) Write(p []byte) (int, error) {
	if ew.err != nil {
		return 0, ew.err
	}
	n, err := ew.Writer.Write(p)
	if err != nil {
		ew.err = err
	}
	return n, err
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("assetfs: ")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] assetsDir ...\n", os.Args[0])
		flag.PrintDefaults()
	}
}

const stub = `
import "net/http"

func AssetDir(name string) http.FileSystem { return http.Dir(name) }
`

const head = `
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
`
