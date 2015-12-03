Command `assetfs` generates http.FileSystem implementation compiling assets
inside go binary as byte slices. Generated file exposes function AssetDir()
which usage is similar to http.Dir() usage.

assetfs can generate 2 files: one for production usage, where assets should be
compiled inside binary, and one for development usage, where AssetDir() is
aliases to http.Dir() and files are served from disk. As both files exposes
function with the same name, they're differentiated with build tags.

	Usage: assetfs [flags] assetsDir ...
	  -dev string
		path to write development stub
	  -devtag string
		build tag to assign to development stub (default "dev")
	  -name string
		package name
	  -out string
		path to write generated content
	  -tag string
		build tag to use for main generated file

This program is mainly intended to be used with `go generate` as follows:

Consider your repository has `static` subdirectory which is normally exposed as
`http.Dir("static")`. Create file `static.go` with the following content:

	package mypackage
	//go:generate assetfs static

Once you run `go generate`, it will produce two extra files:
`static_assetfs.go` and `static_assetfs-dev.go`. Both expose `AssetDir()`
function, the latter has this function aliased to `http.Dir()` and has `dev`
build tag set, so it can be used during development when you expect to see
changes in static files as you refresh page. Next thing to do would be to
replace usage of `http.Dir("static")` with `AssetDir("static")` in your code.

`assetfs` main intention is to include **small** static assets inside compiled
Go binary, so that resulting program has less file dependencies. As all
included files are stored as byte slices, they affect both binary size and
runtime memory consumption, so you should avoid including large files.
Currently the only artificial limit built in is for a single file size (10
MiB).


License: MIT
