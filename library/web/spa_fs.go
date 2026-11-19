package web

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/Laisky/errors/v2"
)

// rootFileSystem is an http.FileSystem confined to dir through os.Root that
// opens regular files only, so neither "..", an escaping symlink, nor a
// directory listing can be served from it.
//
// Every Open re-opens the root, so a rebuilt directory is picked up without a
// restart, matching http.Dir's per-request path resolution.
type rootFileSystem struct {
	dir string
}

// Open opens the regular file name inside f.dir.
//
// Parameters:
//   - name: slash-separated URL path as passed by http.FileServer or gin.
//
// Returns:
//   - http.File: the opened regular file; the caller closes it.
//   - error: wrapped error, matching fs.ErrNotExist for missing files and
//     directories, or the os.Root error for paths that escape f.dir.
func (f rootFileSystem) Open(name string) (http.File, error) {
	rel := strings.TrimPrefix(path.Clean("/"+name), "/")
	if rel == "" {
		rel = "."
	}

	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return nil, errors.Wrapf(err, "open root %q", f.dir)
	}

	file, err := openRegularFile(root, rel)
	if err != nil {
		return nil, errors.Join(err, closeRoot(root))
	}
	if err = closeRoot(root); err != nil {
		return nil, errors.Join(err, errors.Wrap(file.Close(), "close file"))
	}

	return file, nil
}

// openRegularFile opens rel inside root and rejects directories.
//
// Parameters:
//   - root: confinement root.
//   - rel: local, slash-separated path.
//
// Returns:
//   - *os.File: the opened regular file.
//   - error: wrapped open/stat error, or fs.ErrNotExist for a directory.
func openRegularFile(root *os.Root, rel string) (*os.File, error) {
	file, err := root.Open(rel)
	if err != nil {
		return nil, errors.Wrapf(err, "open %q", rel)
	}

	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(errors.Wrapf(err, "stat %q", rel), errors.Wrap(file.Close(), "close file"))
	}
	if info.IsDir() {
		return nil, errors.Join(errors.Wrapf(fs.ErrNotExist, "%q is a directory", rel),
			errors.Wrap(file.Close(), "close file"))
	}

	return file, nil
}

// closeRoot closes root.
//
// Parameters:
//   - root: root to close.
//
// Returns:
//   - error: wrapped close error, or nil.
func closeRoot(root *os.Root) error {
	return errors.Wrap(root.Close(), "close root")
}
