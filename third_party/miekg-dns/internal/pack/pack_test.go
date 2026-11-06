package pack

import (
	"fmt"
	"maps"
	"testing"
)

func TestName(t *testing.T) {
	newmap := func(a ...any) map[string]uint16 {
		m := map[string]uint16{}
		for i := 0; i < len(a); i += 2 {
			m[a[i].(string)] = uint16(a[i+1].(int))
		}
		return m
	}

	testcases := []struct {
		in   string
		ok   bool
		comp map[string]uint16
	}{
		{`www.this.is.an.example.org.`, true, newmap("this.is.an.example.org.", 4, "www.this.is.an.example.org.", 0,
			"an.example.org.", 12, "example.org.", 15, "is.an.example.org.", 9, "org.", 23)},
		{`www.example.org.`, true, newmap("www.example.org.", 0, "example.org.", 4, "org.", 12)},
		{`www.example.org`, false, nil},
		{`org.`, true, newmap("org.", 0)},
		{`.`, true, newmap()},
		{`..`, false, nil},
		{`.org`, false, nil},
		{`www..example.org.`, false, nil},
		{`www.example.org..`, false, nil},
		{``, true, newmap()},
	}
	buf := make([]byte, 256)
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			comp := map[string]uint16{}
			_, got := Name(tc.in, buf, 0, comp, true)
			if (got == nil) != tc.ok {
				t.Errorf("expected %t for name %q: %v", tc.ok, tc.in, got)
			}
			if !tc.ok {
				return
			}
			if !maps.Equal(comp, tc.comp) {
				t.Errorf("expected compression map\n %v, got\n %v", tc.comp, comp)
			}
		})
	}
}

func BenchmarkName(b *testing.B) {
	buf := make([]byte, 256)
	s := "wwww.example.org."
	for b.Loop() {
		Name(s, buf, 0, nil, false)
	}
}

func TestMName(t *testing.T) {
	testcases := []struct {
		in   string
		wire string // empty means MName must return an error
	}{
		{`.`, "\x00"},
		{``, "\x00"},
		{`hostmaster.example.org.`, "\x0ahostmaster\x07example\x03org\x00"},
		{`a\.b.example.org.`, "\x03a.b\x07example\x03org\x00"},
		// The rname of the .mil SOA, which carries four escaped dots in a single label.
		{`DISA\.COLUMBUS\.NS\.MBX\.HOSTMASTER-DOD-NIC.MAIL.mil.`, "\x27DISA.COLUMBUS.NS.MBX.HOSTMASTER-DOD-NIC\x04MAIL\x03mil\x00"},
		{`\\\.a.example.org.`, "\x04\\\\.a\x07example\x03org\x00"},
		{`a\b\.c.example.org.`, "\x05a\\b.c\x07example\x03org\x00"},
		{`a\.b\.c.`, "\x05a.b.c\x00"},
		{`\.a.example.org.`, ""},
		{`.example.org.`, ""},
		{`a\.`, ""},
		{`a\.b.example.org`, ""},
		{`a\.b..example.org.`, ""},
	}
	buf := make([]byte, 256)
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			off, err := MName(tc.in, buf, 0)
			if (err == nil) != (tc.wire != "") {
				t.Fatalf("expected error %t for mname %q, got %v", tc.wire == "", tc.in, err)
			}
			if err != nil {
				return
			}
			if got := string(buf[:off]); got != tc.wire {
				t.Errorf("expected wire %q, got %q", tc.wire, got)
			}
		})
	}
}

func TestStrings(t *testing.T) {
	testcases := []struct {
		in   []string
		size int // length of the buffer given to Strings
		ok   bool
		wire string
	}{
		{[]string{"a", "bc"}, 5, true, "\x01a\x02bc"},
		{[]string{""}, 1, true, "\x00"},
		{[]string{"a", "bc"}, 4, false, ""},
		{[]string{""}, 0, false, ""},
		// No character-strings pack to nothing, also when there is no room left in the buffer.
		{nil, 0, true, ""},
		{[]string{}, 0, true, ""},
	}
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			buf := make([]byte, tc.size)
			off, err := Strings(tc.in, buf, 0)
			if (err == nil) != tc.ok {
				t.Fatalf("expected error %t for strings %q, got %v", !tc.ok, tc.in, err)
			}
			if !tc.ok {
				return
			}
			if got := string(buf[:off]); got != tc.wire {
				t.Errorf("expected wire %q, got %q", tc.wire, got)
			}
		})
	}
}
