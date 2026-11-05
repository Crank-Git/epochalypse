package conffile

// ZoneClass holds the zone and the class, if a class is not set dns.ClassINET is assumed.
type ZoneClass struct {
	Zone  string
	Class uint16
}
