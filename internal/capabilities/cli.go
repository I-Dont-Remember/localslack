package capabilities

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
)

const usage = "usage: capctl docs | gaps"

// Run implements the capctl dev tool. repo is the repository root, write persists
// generated files (relative to the root), and out receives reports.
//
//	docs  regenerate docs/COMPATIBILITY.md
//	gaps  list shipped methods without recorded evidence (the to-do list for recorders)
func Run(args []string, repo fs.FS, write func(name string, data []byte) error, out io.Writer) error {
	if len(args) != 1 {
		return errors.New(usage)
	}
	r, err := BuildReport(repo)
	if err != nil {
		return err
	}
	switch args[0] {
	case "docs":
		return write("docs/COMPATIBILITY.md", []byte(Render(r)))
	case "gaps":
		gaps := r.Gaps()
		_, _ = fmt.Fprintf(out, "%d shipped methods lack recorded evidence (weakest first):\n", len(gaps))
		for _, name := range gaps {
			_, _ = fmt.Fprintf(out, "  %-24s %s\n", name, r.level(name))
		}
		return nil
	}
	return errors.New(usage)
}
