# Synthetic zero-tag key

The `zero.key` and `zero.private` files contain a public synthetic test key pair.
This key provides no confidentiality. Never use it outside tests.
Issue #92 preserves this algorithm 13 fixture with flags 257, protocol 3, and computed key tag zero.
Tests copy the private file to a temporary path with mode `0600` before use.
The resolver fixture retains owner-only access in its container.

RFC 4034 appendix B defines the key tag checksum. Zero is a valid checksum result.
The fixture requires no random search during a test.
