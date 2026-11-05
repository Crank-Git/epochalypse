# Name

_random_ - randomizes the order of address records

# Description

The _random_ handler will act as a "load balancer" by randomizing the order of address (A/AAAA) records in the
answer section.

See [Wikipedia](https://en.wikipedia.org/wiki/Round-robin_DNS) about the pros and cons of this setup. It will
take care to sort any CNAMEs before any address records, because some stub resolver implementations (like
glibc) are particular about that.

# Syntax

```txt
random [POLICY]
```

- **POLICY** is the policy to use, when not given it defaults to `random`.
