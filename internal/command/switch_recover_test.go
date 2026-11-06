package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestSwitchRecoverRequiresPublicRecoveryInputs(t *testing.T) {
	var out, stderr bytes.Buffer
	code := runSwitch([]string{"switch", "recover"}, &out, &stderr)
	if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "--bundle") {
		t.Fatalf("recover inputs: %d %s %s", code, &out, &stderr)
	}
}
