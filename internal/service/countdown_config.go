package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/countdown"
)

func (c Config) validateCountdowns() error {
	if (c.CountdownZone == "") != (c.CountdownKeyFile == "") {
		return fmt.Errorf("set both the countdown zone and DNSSEC key file")
	}
	if c.CountdownZone == "" {
		return nil
	}
	if !countdown.ValidZone(c.CountdownZone) || c.Mode != "server" || c.DNSAddr == "" {
		return fmt.Errorf("countdowns require a canonical zone, the main server, and a DNS address")
	}
	if err := c.validateCountdownSeparation(); err != nil {
		return err
	}
	if err := authority.ValidateNameServer(c.CountdownZone, c.nameServer()); err != nil {
		return err
	}
	if !c.labMaterialAllowed() && (c.NameServer == "" || labZone(c.CountdownZone) || labZone(c.NameServer)) {
		return fmt.Errorf("production countdowns require an explicit external nameserver and a non-lab zone")
	}
	if !c.LocalLab {
		copy := c
		copy.Zone = c.CountdownZone
		return copy.validateProductionAuthority()
	}
	return nil
}

func (c Config) validateCountdownSeparation() error {
	if c.Zone == "" || c.CountdownZone == "" {
		return nil
	}
	windowKey, err := filepath.Abs(c.KeyFile)
	if err != nil {
		return err
	}
	countdownKey, err := filepath.Abs(c.CountdownKeyFile)
	if err != nil {
		return err
	}
	if zoneContains(c.Zone, c.CountdownZone) || zoneContains(c.CountdownZone, c.Zone) || windowKey == countdownKey {
		return fmt.Errorf("countdowns require a separate zone and key reference")
	}
	return nil
}

func zoneContains(zone, name string) bool {
	return name == zone || strings.HasSuffix(name, "."+zone)
}

func (s *mainState) attachCountdowns(config Config) error {
	if err := config.validateCountdownSeparation(); err != nil {
		return err
	}
	var err error
	s.countdowns, err = countdown.Attach(s.content, config.CountdownZone)
	if err != nil {
		return err
	}
	s.countdownSlots = make(chan struct{}, 4)
	if config.CountdownZone == "" {
		return nil
	}
	s.countdownSigner, err = countdown.LoadSigner(config.CountdownZone, config.CountdownKeyFile)
	if err != nil {
		return err
	}
	if s.countdownSigner.LabOnly() && !config.labMaterialAllowed() {
		return errors.New("production countdowns reject an explicit lab key")
	}
	s.countdownAuthority, err = authority.NewCountdown(s.countdowns, s.countdownSigner, config.nameServer())
	return err
}
