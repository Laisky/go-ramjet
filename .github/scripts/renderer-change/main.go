// Command renderer-change detects executable changes to a Go renderer between Git revisions.
package main

import (
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"strings"

	"github.com/Laisky/errors/v2"
)

// main reports the comparison as a GitHub Actions output and exits on invalid inputs.
func main() {
	base := flag.String("base", "", "Baseline Git revision.")
	head := flag.String("head", "", "Head Git revision.")
	path := flag.String("path", "", "Renderer path relative to the repository root.")
	flag.Parse()
	if *base == "" || *head == "" || *path == "" {
		fmt.Fprintln(os.Stderr, "base, head, and path are required")
		os.Exit(1)
	}
	changed, err := rendererChanged(*base, *head, *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("changed=%t\n", changed)
}

// rendererChanged compares meaningful Go tokens from path in the two Git revisions.
func rendererChanged(base, head, path string) (bool, error) {
	baseSource, err := revisionSource(base, path)
	if err != nil {
		return false, errors.Wrap(err, "load baseline renderer")
	}
	headSource, err := revisionSource(head, path)
	if err != nil {
		return false, errors.Wrap(err, "load head renderer")
	}
	baseTokens, err := meaningfulGoTokens(baseSource)
	if err != nil {
		return false, errors.Wrap(err, "scan baseline renderer")
	}
	headTokens, err := meaningfulGoTokens(headSource)
	if err != nil {
		return false, errors.Wrap(err, "scan head renderer")
	}
	return baseTokens != headTokens, nil
}

// revisionSource reads path from a Git revision without checking it out or executing it.
func revisionSource(revision, path string) ([]byte, error) {
	source, err := exec.Command("git", "show", revision+":"+path).Output()
	if err != nil {
		return nil, errors.Wrapf(err, "read renderer at %s", revision)
	}
	return source, nil
}

// meaningfulGoTokens ignores whitespace and ordinary comments while retaining compiler directives.
func meaningfulGoTokens(source []byte) (string, error) {
	files := token.NewFileSet()
	file := files.AddFile("renderer.go", -1, len(source))
	var scan scanner.Scanner
	var scanErrors []string
	scan.Init(file, source, func(position token.Position, message string) {
		scanErrors = append(scanErrors, fmt.Sprintf("%s: %s", position, message))
	}, scanner.ScanComments)
	var result strings.Builder
	pendingSemicolon := false
	for {
		_, kind, literal := scan.Scan()
		if kind == token.EOF {
			if pendingSemicolon {
				fmt.Fprintf(&result, "%d:%q\n", token.SEMICOLON, "\n")
			}
			break
		}
		if kind == token.COMMENT {
			comment := strings.TrimSpace(literal)
			if !strings.HasPrefix(comment, "//go:") &&
				!strings.HasPrefix(comment, "// +build ") &&
				!strings.HasPrefix(comment, "//line ") &&
				!strings.HasPrefix(comment, "/*line ") {
				continue
			}
		}
		if kind == token.SEMICOLON && literal == "\n" {
			pendingSemicolon = true
			continue
		}
		// Go permits omission of a semicolon immediately before a closing brace.
		// Formatting can insert one without changing execution.
		if pendingSemicolon && kind != token.RBRACE {
			fmt.Fprintf(&result, "%d:%q\n", token.SEMICOLON, "\n")
		}
		pendingSemicolon = false
		fmt.Fprintf(&result, "%d:%q\n", kind, literal)
	}
	if len(scanErrors) > 0 {
		return "", errors.Errorf("invalid Go source: %s", strings.Join(scanErrors, "; "))
	}
	return result.String(), nil
}
