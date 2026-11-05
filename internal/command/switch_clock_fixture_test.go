package command

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/agent"
)

func writeSwitchClockProfile(t *testing.T, root string) *agent.ClockProfile {
	t.Helper()
	path := filepath.Join(root, "chronyc-fixture")
	ref := time.Now().UTC().Format("Mon Jan _2 15:04:05 2006")
	output := "#!/bin/sh\ncase \"$*\" in\n'-v') cat <<'END'\nchronyc (chrony) version 4.9 (+READLINE)\nEND\n;;\n'-n tracking') cat <<'END'\nReference ID : CB00710F (192.0.2.1)\nRef time (UTC) : " + ref + "\nSystem time : 0.000001 seconds slow of NTP time\nSkew : 0.1 ppm\nRoot delay : 0.001 seconds\nRoot dispersion : 0.001 seconds\nLeap status : Normal\nEND\n;;\n'-n sources') cat <<'END'\n^* 192.0.2.1 2 6 377 0 -4us[ -6us] +/- 1ms\n^+ 198.51.100.2 2 6 377 0 -4us[ -6us] +/- 1ms\nEND\n;;\n*) exit 1;;\nesac\n"
	writeFixtureFile(t, path, []byte(output), 0o700)
	return &agent.ClockProfile{Executable: path, Version: "4.9", DaemonVersion: "4.9", OperatorVerified: true,
		Provenance: "synthetic chronyc output with real Linux capture", Sources: []string{"192.0.2.1", "198.51.100.2"},
		ReferenceEvidence: "synthetic reference premise", IndependenceEvidence: "synthetic independence premise", LeapPolicy: "UTC normal only",
		BaseUncertainty: 100 * time.Millisecond, RatePPM: 100}
}
