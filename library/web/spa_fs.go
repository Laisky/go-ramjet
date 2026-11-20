package web

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"syscall"

	"github.com/Laisky/errors/v2"
)

// rootFileSystem is an http.FileSystem confined to dir through os.Root that
// opens regular files only, so neither "..", an escaping symlink, nor a
// directory listing can be served from it.
//
// Every Open re-opens the root, so a rebuilt directory is picked up without a
// restart, matching http.Dir's per-request path resolution.
type rootFileSystem struct {
	dir    string
	prefix string // optional path inside dir; never becomes the root
}

// Open opens the regular file name inside f.dir.
//
// Parameters:
//   - name: slash-separated URL path as passed by http.FileServer or gin.
//
// Returns:
//   - http.File: the opened regular file; the caller closes it.
//   - error: wrapped error, matching fs.ErrNotExist for missing files,
//     directories and every path os.Root refuses (see notFoundIfRefused).
func (f rootFileSystem) Open(name string) (http.File, error) {
	// Rebase the cleaned HTTP path within the mounted prefix, but retain
	// dir as the root used to resolve all of its components.
	rel := path.Join(f.prefix, strings.TrimPrefix(path.Clean("/"+name), "/"))
	if rel == "" {
		rel = "."
	}

	root, err := f.openRoot()
	if err != nil {
		return nil, err
	}

	file, err := openRegularFile(root, rel)
	if err != nil {
		return nil, errors.Join(notFoundIfRefused(err), closeRoot(root))
	}
	if err = closeRoot(root); err != nil {
		return nil, errors.Join(err, errors.Wrap(file.Close(), "close file"))
	}

	return file, nil
}

// openRoot opens the distribution directory with the shared confinement policy.
func (f rootFileSystem) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return nil, errors.Wrapf(err, "open root %q", f.dir)
	}
	return root, nil
}

// openRegularFile opens rel inside root and rejects non-regular files.
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
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.Wrapf(fs.ErrNotExist, "%q is not a regular file", rel),
			errors.Wrap(file.Close(), "close file"))
	}

	return file, nil
}

// notFoundIfRefused marks the errors os.Root returns for paths it refuses to
// resolve (a symlink escaping the root or an absolute one, a symlink loop, a
// path through a regular file) as fs.ErrNotExist, so http.FileServer answers
// 404 for them instead of 500. This is stricter than http.Dir, which only maps
// a path through a regular file to not found, follows escaping symlinks, and
// answers 500 for symlink loops and for its own unsafe-path error.
//
// Parameters:
//   - err: error from opening a path inside an os.Root.
//
// Returns:
//   - error: err, joined with fs.ErrNotExist when it is such a refusal.
//     Permission errors (403) and I/O or resource errors such as EMFILE
//     (500) are returned unchanged.
func notFoundIfRefused(err error) error {
	var errno syscall.Errno
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return err
	case errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.ELOOP):
		return errors.Join(err, fs.ErrNotExist)
	case errors.As(err, &errno):
		return err
	default:
		// os.Root's own "path escapes from parent" refusal carries no errno.
		return errors.Join(err, fs.ErrNotExist)
	}
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
