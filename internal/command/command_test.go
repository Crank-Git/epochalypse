package command

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRunShowsHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage:") || errOut.Len() != 0 {
			t.Fatalf("Run(%q): code=%d stdout=%q stderr=%q", args, code, &out, &errOut)
		}
	}
}

func TestRunRejectsUnknownArguments(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"help", "extra"}, {"--bad"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "invalid command") {
			t.Fatalf("Run(%q): code=%d stdout=%q stderr=%q", args, code, &out, &errOut)
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRunRejectsOutputFailure(t *testing.T) {
	if code := Run(nil, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if code := Run([]string{"bad"}, &bytes.Buffer{}, failedWriter{}); code != 1 {
		t.Fatalf("error exit code = %d, want 1", code)
	}
}

func FuzzRun(f *testing.F) {
	f.Add("help")
	f.Add("unknown")
	f.Fuzz(func(t *testing.T, arg string) {
		var out, errOut bytes.Buffer
		code := Run([]string{arg}, &out, &errOut)
		if code != 0 && code != 2 {
			t.Fatalf("exit code = %d", code)
		}
	})
}
