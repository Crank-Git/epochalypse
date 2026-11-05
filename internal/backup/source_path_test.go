package backup

import (
	"bytes"
	"context"
	"filippo.io/age"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveRejectsLinkedSourceDirectories(t *testing.T) {
	a := fixture(t)
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(filepath.Dir(a.Files[0].SourcePath), link); err != nil {
		t.Fatal(err)
	}
	a.Files[0].SourcePath = filepath.Join(link, filepath.Base(a.Files[0].SourcePath))
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var dst bytes.Buffer
	if err := Seal(context.Background(), &dst, a, identity.Recipient(), testLimits(t)); err == nil || dst.Len() != 0 {
		t.Fatal("linked source directory reaches encryption")
	}
}
