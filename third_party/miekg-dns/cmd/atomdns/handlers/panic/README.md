# Name

_panic_ - recover from panics

# Description

_panic_ recovers from a panic.

# Syntax

```txt
panic
```

# Examples

```conffile
example.org {
    panic
    any
    whoami
}
```

# Metrics

If monitoring is enabled (via the _metrics_ handler) then the following metrics are exported:

- `atomdns_panic_panics_total{}` - counter of recovered panics.
