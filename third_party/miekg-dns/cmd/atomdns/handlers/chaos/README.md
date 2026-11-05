# Name

_chaos_ - respond to TXT queries in the CH class

# Description

This is useful for retrieving version or author information from the server by querying a TXT record
for a special domain name in the CH class. For this to works you have to signal you need CH class queries in
the config file, by adding **/CH** to the zone, see atomdns-conffile(5).

# Syntax

```
chaos [VERSION] {
    authors {
        "First Author"
        "Second Author"
        ...
    }
}
```

- **VERSION** is the version to return. Defaults to "Served by atomdns, https://atomdns.miek.nl" if not set.
- The `authors` property holds the authors that are returned.

Note that you have to make sure that this handler will get actual queries for
the following zone _prefixes_: `version.`, `authors.`, `hostname.` and `id.`,
i.e. having `version.example.org` will suffice to get queries for the
`version.` prefix.

# Examples

Specify all the zones:

```conffile
version.bind/CH version.server/CH authors.bind/CH hostname.bind/CH id.server/CH authors.server/CH {
    chaos atomdns-001 {
        authors {
            info@example.org
        }
    }
}
```

And test with `dig`:

```txt
% dig @localhost CH TXT version.bind

;; ANSWER SECTION:
version.bind.		0	CH	TXT	"atomdns-001"
```

# See Also

atomdns-conffile(5).
