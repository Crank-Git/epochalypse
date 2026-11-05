package url

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnslog"
)

func (u *Url) Refetch() error {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				err := u.Fetch()
				if err != nil {
					alog := log().With("URLs", dnslog.GroupValues("URL", u.URLs), slog.String("file", filepath.Base(u.Path)))
					alog.Error("Failed to fetch", Err(err))
					continue
				}
			case <-u.ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (u *Url) Fetch() error {
	c := http.Client{Timeout: 10 * time.Second}
	for _, url := range u.URLs {
		resp, err := c.Get(url)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			continue
		}
		lr := &io.LimitedReader{R: resp.Body, N: 8 * 1024 * 1024}
		body, err := io.ReadAll(lr)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			continue
		}
		resp.Body.Close()

		zp := dns.NewZoneParser(bytes.NewReader(body), ".", "-")
		zp.SkipDirectives = true
		for _, ok := zp.Next(); ok; _, ok = zp.Next() {
		}
		if zp.Err() != nil {
			return fmt.Errorf("failed to parse zone %s with origin %s: %s ", "-", ".", zp.Err())
		}

		f, err := os.CreateTemp(filepath.Dir(u.Path), "xxxxx.transferred")
		if err != nil {
			return err
		}
		if err := os.WriteFile(f.Name(), body, 0600); err != nil {
			return err
		}
		defer f.Close()
		defer os.Remove(f.Name())
		return os.Rename(f.Name(), u.Path)
	}
	return fmt.Errorf("failed to fetch from all URLs")
}
