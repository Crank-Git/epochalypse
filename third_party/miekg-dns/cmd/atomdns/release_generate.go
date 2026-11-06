//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"regexp"
	"time"
)

var DebianChangelog = template.Must(template.New("DebianChangelog").Funcs(template.FuncMap{
	"safe": func(s string) template.HTML { return template.HTML(s) },
}).Parse(`atomdns ({{.Version}}) unstable; urgency=medium

  * New upstream release

 -- Miek Gieben <miek@miek.nl>  {{.Time.Format "Mon, 02 Jan 2006 15:04:05 -0700" | safe}}
`))

func main() {
	// generate debian changelog:
	//
	// atomdns (058) unstable; urgency=medium
	//
	//   * New upstream release
	//
	//  -- Miek Gieben <miek@miek.nl>  Thu, 18 Dec 2025 10:57:35 +0100
	buf, err := os.ReadFile("atomdns.go")
	if err != nil {
		log.Fatal(err)
	}
	version := Version(string(buf))

	changelog, err := os.OpenFile("_release/debian/changelog", os.O_RDWR|os.O_TRUNC, 0640)
	if err != nil {
		log.Fatal(err)
	}
	defer changelog.Close()
	type wrap struct {
		Version string
		Time    time.Time
	}

	w := wrap{Version: version, Time: time.Now().UTC()}
	if err := DebianChangelog.Execute(changelog, w); err != nil {
		log.Fatal(err)
	}

	// Version: is in the first two lines
	rpmbuf, err := os.ReadFile("_release/fedora/atomdns.spec")
	if err != nil {
		log.Fatal(err)
	}
	first := bytes.Index(rpmbuf, []byte("\n"))
	second := bytes.Index(rpmbuf[first+1:], []byte("\n"))
	header := fmt.Sprintf("Name:\tatomdns\nVersion:\t%s", version)
	buf = append([]byte(header), rpmbuf[first+second+1:]...)

	err = os.WriteFile("_release/fedora/atomdns.spec", buf, 0640)
	if err != nil {
		log.Fatal(err)
	}
}

func Version(source string) string {
	re := regexp.MustCompile(`const\s+version\s*=\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(source)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
