package dbsqlite

import (
	"strconv"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnszone"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/miekg/dns/pkg/pool"
	"github.com/jmoiron/sqlx"
)

type Zone struct {
	db     *sqlx.DB
	origin string
	labels int
}

var _ dnszone.Interface = &Zone{}

// RR is the data we stored in the rrs table.
type RR struct {
	Name string
	Type string
	Data string
	TTL  int
	Date time.Time
}

func (z *Zone) Load(dnszone.LoadOptions) error { return nil }
func (z *Zone) Set(*dnszone.Node) string       { return "" }
func (z *Zone) Origin() string                 { return z.origin }
func (z *Zone) Labels() int                    { return z.labels }

func (z *Zone) Walk(fn func(*dnszone.Node) bool) {
	// For some reason this give no names, but:
	// err := z.db.Select(&names, `SELECT DISTINCT name FROM rrs WHERE name LIKE '%.?' COLLATE canonical`, z.origin)
	// this does:
	names := []string{}
	err := z.db.Select(&names, `SELECT DISTINCT name FROM rrs WHERE name LIKE %%.? COLLATE canonical OR name = ? ORDER BY name COLLATE canonical`, z.origin, z.origin)
	if err != nil {
		return
	}
	for _, name := range names {
		n, ok := z.Get(name)
		if !ok {
			continue
		}
		if !fn(n) {
			return
		}
	}
}

func (z *Zone) AuthoritativeWalk(fn func(*dnszone.Node, bool) bool) {
	// See comment in Walk, we keep track of delegations, also see dbfile/zone.Walk.
	names := []string{}
	err := z.db.Select(&names, `SELECT DISTINCT name FROM rrs WHERE name LIKE %%.? COLLATE canonical OR name = ? ORDER BY name COLLATE canonical`, z.origin, z.origin)
	if err != nil {
		return
	}

	delegated := map[string]struct{}{}

	z.Walk(func(n *dnszone.Node) bool {
		if len(n.Name) > len(z.Origin()) { // apex also has NSes, if we add those the entire zone would be delegated
			for _, rr := range n.RRs {
				if _, ok := rr.(*dns.NS); ok {
					delegated[n.Name] = struct{}{}
					break
				}
			}
		}
		auth, end := true, false
		i, j := 0, 0
		for ; !end; j, end = dnsutil.Next(n.Name, i) {
			if len(n.Name[j:]) < len(z.Origin()) {
				break
			}
			if _, ok := delegated[n.Name[j:]]; ok {
				auth = false
				break
			}
			i++
		}

		return fn(n, auth)
	})
}

func (z *Zone) Previous(name string) *dnszone.Node {
	prevs := []string{}
	err := z.db.Select(&prevs, "SELECT name FROM rrs WHERE name < ? COLLATE canonical ORDER BY name COLLATE canonical DESC LIMIT 1", name)
	if err != nil || len(prevs) == 0 {
		return nil
	}
	node, _ := z.Get(prevs[0])
	return node
}

func (z *Zone) Get(name string) (*dnszone.Node, bool) {
	// Get will get name, if that doesn't return anything we do LIKE '%.<name>' this is to shake out empty
	// non-terminals. If we have something returned we know that <name> is an ENT. Wildcards are handled by
	// retrieve.

	rrs := []RR{}
	err := z.db.Select(&rrs, "SELECT * FROM rrs WHERE name = ?", name)
	if err != nil {
		return nil, false
	}
	sb := builderPool.Get()

	if len(rrs) > 0 {
		node := &dnszone.Node{Name: name, RRs: make([]dns.RR, 0, len(rrs))}
		for _, rr := range rrs {
			sb.WriteString(rr.Name)
			sb.WriteByte(' ')
			sb.WriteString(strconv.Itoa(rr.TTL))
			sb.WriteByte(' ')
			sb.WriteString(rr.Type)
			sb.WriteByte(' ')
			sb.WriteString(rr.Data)
			sb.WriteByte('\n')
			rr1, err := dns.New(sb.String())
			if err != nil {
				log().Debug("Failed to convert to dns.RR", "rr", sb, Err(err))
				sb.Reset()
				continue
			}
			node.RRs = append(node.RRs, rr1)
			sb.Reset()
		}
		sb.Reset()
		builderPool.Put(sb)
		return node, true
	}

	// nothing found, check for empty non-terminals, if this returns a wildcard? Should we exclude wildcards? TODO(miek).
	names := []string{}
	err = z.db.Select(&names, `SELECT DISTINCT name FROM rrs WHERE name LIKE '%%.?' COLLATE canonical`, name)
	if err != nil {
		return nil, false
	}
	if len(names) > 0 { // we have found longer names than name, we have an empty non-terminal at name
		return &dnszone.Node{Name: name}, true
	}

	return nil, false
}

func (z *Zone) Apex() *dnszone.Node {
	node, _ := z.Get(z.Origin())
	if node != nil {
		return node
	}
	return nil
}

var builderPool = pool.NewBuilder()
