package authority

import "codeberg.org/miekg/dns"

func wireNamePolicy(data []byte) uint16 {
	// The codec loses label boundaries for literal dots and cannot preserve arbitrary octets.
	offset, size := dns.MsgHeaderSize, 1
	for range 128 {
		if offset >= len(data) {
			return dns.RcodeFormatError
		}
		length := int(data[offset])
		offset++
		if length&0xc0 == 0xc0 {
			if offset >= len(data) {
				return dns.RcodeFormatError
			}
			offset = (length&0x3f)<<8 | int(data[offset])
			continue
		}
		if length&0xc0 != 0 {
			return dns.RcodeFormatError
		}
		if length == 0 {
			return dns.RcodeSuccess
		}
		size += length + 1
		if size > 255 || offset+length > len(data) {
			return dns.RcodeFormatError
		}
		if !supportedLabel(data[offset : offset+length]) {
			return dns.RcodeRefused
		}
		offset += length
	}
	return dns.RcodeFormatError
}

func supportedLabel(label []byte) bool {
	for _, octet := range label {
		if octet < 33 || octet > 126 || octet == '.' || octet == '\\' {
			return false
		}
	}
	return true
}
