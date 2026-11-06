package dns

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/mldsa"
	"crypto/rsa"
	"testing"
)

func TestDNSSECSignVerify(t *testing.T) {
	// Add wildcard, sorting of RRs. etc.
	testcases := []struct {
		name      string
		algorithm uint8
		bitsize   int
		rrs       []RR
	}{
		{
			"rsasha256", RSASHA256, 1024,
			[]RR{
				&SRV{Hdr: Header{Name: "srv.miek.nl.", Class: ClassINET, TTL: 600}, Port: 1000, Weight: 80, Target: "web1.miek.nl."},
			},
		},
		{
			"ecdsap256sha256", ECDSAP256SHA256, 256,
			[]RR{
				&SRV{Hdr: Header{Name: "srv.miek.nl.", Class: ClassINET, TTL: 600}, Port: 1000, Weight: 80, Target: "web1.miek.nl."},
			},
		},
		{
			"ed25519", ED25519, 256,
			[]RR{
				&SRV{Hdr: Header{Name: "srv.miek.nl.", Class: ClassINET, TTL: 600}, Port: 1000, Weight: 80, Target: "web1.miek.nl."},
			},
		},
		{
			"mldsa44", MLDSA44, 256,
			[]RR{
				&SRV{Hdr: Header{Name: "srv.miek.nl.", Class: ClassINET, TTL: 600}, Port: 1000, Weight: 80, Target: "web1.miek.nl."},
			},
		},
		{
			"rsasha256-sorting", RSASHA256, 1024,
			[]RR{
				&NS{Hdr: Header{Name: "miek.nl.", Class: ClassINET, TTL: 600}, Ns: "linode.atoom.net."},
				&NS{Hdr: Header{Name: "miek.nl.", Class: ClassINET, TTL: 600}, Ns: "ns-ext.nlnetlabs.nl."},
				&NS{Hdr: Header{Name: "miek.nl.", Class: ClassINET, TTL: 600}, Ns: "omval.tednet.nl"},
			},
		},
	}

	options := &SignOption{}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var err error

			key := NewDNSKEY("miek.nl.", tc.algorithm)
			priv, _ := key.Generate(tc.bitsize)

			sig := NewRRSIG("miek.nl.", tc.algorithm, key.KeyTag())
			switch tc.algorithm {
			case RSASHA256:
				err = sig.Sign(priv.(*rsa.PrivateKey), tc.rrs, options)
			case ECDSAP256SHA256:
				err = sig.Sign(priv.(*ecdsa.PrivateKey), tc.rrs, options)
			case ED25519:
				err = sig.Sign(priv.(ed25519.PrivateKey), tc.rrs, options)
			case MLDSA44:
				err = sig.Sign(priv.(*mldsa.PrivateKey), tc.rrs, options)
			}
			if err != nil {
				t.Fatalf("failure to sign: %s", err)
			}

			err = sig.Verify(key, tc.rrs, options)
			if err != nil {
				t.Fatalf("failure to verify: %s", err)
			}
		})
	}
}

func TestDNSSECKeyTag(t *testing.T) {
	testcases := []struct {
		tag uint16
		rr  RR
	}{
		{
			10771, dnstestNew("example.net. 3600 IN DNSKEY 257 3 14 xKYaNhWdGOfJ+nPrL8/arkwf2EY3MDJ+SErKivBVSum1w/egsXvSADtNJhyem5RCOpgQ6K8X1DRSEkrbYQ+OB+v8/uX45NBwY8rp65F6Glur8I/mlVNgF6W/qTI37m40"),
		},
		{
			59829, dnstestNew("example.com.	3600 IN	DNSKEY 257 3 18 17K0clSq4NtF55MNSpjSyX2PE5fReJ2voXAksxbpvslPyZRtQvGbeadBO7qjPnFJy0LtURVpOsBB+suYit61/g4dhjEYSZW1ksOX0ilOLhT5CqQUujgmiZrEP0zMrLwm6agyuVEY1ctDPL75ZgsAE44IF/YediyidMNq1VTrIqrBFi5KsBrLoeOMTv2PgLZbMz0PcuVd/nHOnB67mInnxWEGwP1zgDoq7P6v3teqPLLO2lTRK9jNNqeM+XWUO0er0l6ICsRS5XQu0ejRqCr6huWQx1jBWuTShA2SvKGlCQ9ASWWX/KfYuVE/GhvabpUKqpjeRnUH1KT1pPBZkhZYLDVy9i7aiQWrNYFnDEoCd3oz4Mpylf2PT/bRoKOnaD1l9fX3/GDaAj6CbF+SFEwC99G6EHWYdVPqk2f8122ZC3+pnNRa/biDbUPkWfUYffBYR5cJoB6mg1k1+nBGCZDNPcG6QBupS6sd3kGsZ6szGdysoGBI1MTu8n7hOpwX0FOPQw8tZC3CQVZg3niHfY2KvHJSOXjAQuQoX0MZhGxEEmJCl2hEwQ5Va6IVtacZ5Z0MayqW05hZBx/cws3nUkp77a5U6FsxjoVOj+Ky8+36yXGRKCcKr9HlBEw6T9r9n/MfkHhLjo5FlhRKDa9YZRHT2ZYrnqla8Ze05fxg8rHtFd46W+9fib3HnZEFHZsoFudPpUUx79wcvnTUSIV/R2vNWPIcC2U7O3ak4HamVZowJxhVXMY/dIWaq6uSXwI4YcqM0Pe62yhx9n1VMm10URNa1F9KG6aRGPuyyKMO7JOS7z+XcGbJrdXHEMxkexUU0hfZWMcBfD6Q/SDATmdLkEhuk3CjGgAdMvRzl55JBnSefkd/oLdFCPil8jeDErg8Jb04jKCw//dHi69CtxZn7arJfEaxKWQ+WG5bBVoMIRlG1PNuZ1vtWGD6BCoxXZgmFk1qkjfDWl+/SVSQpb1N8ki5XEqud4S2BWcxZqxCRbW0sIKgnpMj5i8geMW3Z4NEbe/XNq06NwLUmwiYRJAKYYMzl7xEGbMNepegs4fBkRR0xNQbU+Mql3rLbw6nXbZbs55Z5wHnaVfe9vLURVnDGncSK1IE47XCGfFoixTtC8C4AbPm6C3NQ+nA6fQXRM2YFb0byIINi7Ej8E+s0bG2hd1aKxuNu/PtkzZw8JWhgLTxktCLELj6u9/MKyRRjjLuoKXgyQTKhEeACD87DNLQuLavZ7w1W5SUAl3HsKePqA46Lb/rUTKIUdYHgZjpSTZRrnh+wCUfkiujDp9R32Km1yeEzz3SBTkxdt+jJKUSvZSXCjbdNKUUqGeR8Os28BRbCatkZRtKAxOymWEaKhxIiRYnWYdooxFAYLpEQ0ht9RUioc6IswmFwhb45u0XjdVnswSg1Mr7qIKig0LxepqiauWNtjAIPSw1j99WbD9dYqQoVnvJ6ozpXKoPNUdLC/qPM5olCrTfzyCDvo7vvBBV4Y/hU3DuyyYFZtg/8GshGq7EPKKbVMzQD4gVokZe8LRlFcx+QfMSTwnv/3OTCatYspoUWaALzlA46TjJZ49y6w5O5f2q5m2fhXP8l/xCtJWfS/i2HXhDPoawM11ukZHE2L9IezkFwQjP1qwksM633LfPUfhNDtaHuV6uscUzwG8NlwI9kqcIJYN7Wbpst9TlawqHwgOGKujzFbpZJejt76Z5NpoiAnZhUfFqll+fgeznbMBwtVhp5NuXhM8FyDCzJCyDEg=="),
		},
	}
	for i, tc := range testcases {
		got := tc.rr.(*DNSKEY).KeyTag()
		if got != tc.tag {
			t.Errorf("test %d, expected %d, got %d", i, tc.tag, got)
		}
	}
}

func TestDNSSECToDS(t *testing.T) {
	key := dnstestNew("example.net. 3600 IN DNSKEY 257 3 14 xKYaNhWdGOfJ+nPrL8/arkwf2EY3MDJ+SErKivBVSum1w/egsXvSADtNJhyem5RCOpgQ6K8X1DRSEkrbYQ+OB+v8/uX45NBwY8rp65F6Glur8I/mlVNgF6W/qTI37m40").(*DNSKEY)
	for _, h := range []uint8{SHA1, SHA256, SHA384} {
		if key.ToDS(h) == nil {
			t.Errorf("digest %d (%s), expected a DS", h, HashToString[h])
		}
	}
	for _, h := range []uint8{GOST94, GOST2012, SM3} {
		if key.ToDS(h) != nil {
			t.Errorf("digest %d (%s), expected no DS", h, HashToString[h])
		}
	}
}

func TestDNSSECVerify(t *testing.T) {
	testcases := []struct {
		name string
		key  *DNSKEY
		sig  *RRSIG
		rrs  []RR
	}{
		{
			"root",
			&DNSKEY{Hdr: Header{Name: ".", Class: ClassINET, TTL: 172800}, Flags: 256, Protocol: 3, Algorithm: RSASHA256,
				PublicKey: "AwEAAbauxLSFZ+KSWi2cT6TJbm3d+GIVqb2N1XnDjMsRme0b6JlGp/cvwmM5CaJ5LQ7tG1r7LuTHjYZadtbNk2nZmclq9r4KInS48ungoAZb0gJXVw8IvBTBb1YWQmiBqD285pJuORwTii7DF++nNJJk3i55HJt9SmBI7m7t8nvx7OOY/w0inxg3fLH2uY0SKO8he4FGwMc4Ubiab8N8Yhyhh+FkKKdD/+oAcuGF75PjlSXO460B4MlNLlEcjDEzIsKauRYx4YVgSaNomGhMMFblmXRzgW+1R6ywvm5mC9+omlyyizZp2GJfPwGMezuKSGDndO6CYYEc5/lsRhvBYsGjdPM=",
			},
			&RRSIG{Hdr: Header{Name: ".", Class: ClassINET, TTL: 86400}, TypeCovered: TypeSOA, Algorithm: RSASHA256, Expiration: 1760245200, Inception: 1759118400, OrigTTL: 86400, KeyTag: 46441, SignerName: ".",
				Signature: "Bi335z6iBqX1BsA6AUc29BdgoVcn2a6hfoowvC0OqNbQ1XdIz6lJ6L7iXPBYwUXTP85yII2wIBhIQJSKKuBnGmYwgXsRR7d6vxbcC63UKp6lG/xjMzae2TvuQINQhMCE2N+ufN4DCmlZspdDmWOyPkemJhncrWA+V6GGWLbCtVXPEbcgAvaNyVtFlLp788SdoDy1rpvIM2ZI9tGM2gxZG7wMLPIODtdl865H87kHaHbBkfKkCHsVCeWANoW9r8usdgt8+2HHf3w67HTERUv2NN31fgdkezaS1/7LMaMUb+5mqLygIJPVXbmK8wiUbnyuYW6Ems+R1FEnc2Dd5x5kAw==",
			},
			[]RR{&SOA{Hdr: Header{Name: ".", Class: ClassINET, TTL: 86400}, Ns: "a.root-servers.net.", Mbox: "nstld.verisign-grs.com.", Serial: 2025092900, Refresh: 1800, Retry: 900, Expire: 604800, Minttl: 86400}},
		},
		{
			"org",
			&DNSKEY{Hdr: Header{Name: "org.", Class: ClassINET, TTL: 3600}, Flags: 257, Protocol: 3, Algorithm: RSASHA256,
				PublicKey: "AwEAAexZJ/1wfyNCxNPrTZizaG7UlibGhP+AyogR6bqjptKweEgE4gD8GxRQJkt+Fn5pCoNqzmm1ZnEoKqvm93uOYtbKkYQDGH+W69J66MSKpgIyS+mT/4iaXn+lpb5o99l/sf7lHMa975O/fqN6aPUll4hUbN2T1LHv6HzQuQCtNRJA8jHGwX5q0NMmh2Z+yaG6B9cISerje9l5L+ID2ydJ6zXquYteoIUvX2xzqnXCdHPSvD+oL6R/weW+tztdFS1hok/1z3tn5NzmcaOLll9nXniCozEpLFEGPswyvtphWgCYhI8bBTqhUsIwfIwLSBQTEg2oCX7sS5CbXg44OqwhIW8=",
			},
			&RRSIG{Hdr: Header{Name: "org.", Class: ClassINET, TTL: 3600}, TypeCovered: TypeDNSKEY, Algorithm: RSASHA256, Labels: 1, Expiration: 1770478705, Inception: 1768660705, OrigTTL: 3600, KeyTag: 26974, SignerName: "org.",
				Signature: "YFVPRIIx6ZItt/17yrZmtnBQOFRD44rNDwOh3BQC+NhaM7+6w5kVVpMbVMFy6X8yuXp3+A857I6g6FYVB2p7zDRhq1hkIRyxyYKMmyQmgd0d/Km+vYU+KQjSDWFp0Cm7B+3q5bvbZKRsWho36fofO37jyWkDKYl3tPm8hSCzNuCJ7NfH+3GpcztYL/M3xeHJSJ1wwzFZUW7ioAY4cnmdzHEraXi1O/2UKb+h0lR6CdvNGSe6FLsmx1OETQ8JKneopXpm3RG07AwMSMXw+lgo1d0DZiXwscJpcqHWm9eWaI7OQX6JFl96Yjnjjh1z8gLXFYMXDxLUB3wBtYF903ukiQ==",
			},
			[]RR{
				&DNSKEY{Hdr: Header{Name: "org.", Class: ClassINET, TTL: 3600}, Flags: 256, Protocol: 3, Algorithm: RSASHA256,
					PublicKey: "AwEAAfCN1yguCELJYujNmis5cjZFEW4UcxwSitTh7m1RYMEHTjhSCCzeMZ+UrpNIxLDLwAtWCcAQPoLuQdbhbMBo17pkK2UF26k+WKLN4Ieyw8SsAFScQkf+K7wCYJ4fRJgpsvZgRYBwzpsfQjm26Jd8B7olV02AnvxUHuPo1QeGz0Dd",
				},
				&DNSKEY{Hdr: Header{Name: "org.", Class: ClassINET, TTL: 3600}, Flags: 256, Protocol: 3, Algorithm: RSASHA256,
					PublicKey: "AwEAAa0ig3MNvgtoYj85frPVdYmRm98PlHMHxQBQRRsGC4wucf5PHv9wlSQOJUfGXY45KlJpdgZX8B6s4vHZwOGW2utqp9u2JLMFaMb5bhZUlbD/qufIQu8hcLtoTkORdaK0lo+9vx8R+N2I0DcIVVWBjtSoNjb5L/ssPPicUsBobl2B",
				},
				&DNSKEY{Hdr: Header{Name: "org.", Class: ClassINET, TTL: 3600}, Flags: 257, Protocol: 3, Algorithm: RSASHA256,
					PublicKey: "AwEAAexZJ/1wfyNCxNPrTZizaG7UlibGhP+AyogR6bqjptKweEgE4gD8GxRQJkt+Fn5pCoNqzmm1ZnEoKqvm93uOYtbKkYQDGH+W69J66MSKpgIyS+mT/4iaXn+lpb5o99l/sf7lHMa975O/fqN6aPUll4hUbN2T1LHv6HzQuQCtNRJA8jHGwX5q0NMmh2Z+yaG6B9cISerje9l5L+ID2ydJ6zXquYteoIUvX2xzqnXCdHPSvD+oL6R/weW+tztdFS1hok/1z3tn5NzmcaOLll9nXniCozEpLFEGPswyvtphWgCYhI8bBTqhUsIwfIwLSBQTEg2oCX7sS5CbXg44OqwhIW8=",
				},
			},
		},
		{
			"org-from-string",
			dnstestNew("org.	30	IN	DNSKEY	257 3 8 AwEAAexZJ/1wfyNCxNPrTZizaG7UlibGhP+AyogR6bqjptKweEgE4gD8GxRQJkt+Fn5pCoNqzmm1ZnEoKqvm93uOYtbKkYQDGH+W69J66MSKpgIyS+mT/4iaXn+lpb5o99l/sf7lHMa975O/fqN6aPUll4hUbN2T1LHv6HzQuQCtNRJA8jHGwX5q0NMmh2Z+yaG6B9cISerje9l5L+ID2ydJ6zXquYteoIUvX2xzqnXCdHPSvD+oL6R/weW+tztdFS1hok/1z3tn5NzmcaOLll9nXniCozEpLFEGPswyvtphWgCYhI8bBTqhUsIwfIwLSBQTEg2oCX7sS5CbXg44OqwhIW8=").(*DNSKEY),
			dnstestNew("org.	30	IN	RRSIG	DNSKEY 8 1 3600 20260207153825 20260117143825 26974 org. YFVPRIIx6ZItt/17yrZmtnBQOFRD44rNDwOh3BQC+NhaM7+6w5kVVpMbVMFy6X8yuXp3+A857I6g6FYVB2p7zDRhq1hkIRyxyYKMmyQmgd0d/Km+vYU+KQjSDWFp0Cm7B+3q5bvbZKRsWho36fofO37jyWkDKYl3tPm8hSCzNuCJ7NfH+3GpcztYL/M3xeHJSJ1wwzFZUW7ioAY4cnmdzHEraXi1O/2UKb+h0lR6CdvNGSe6FLsmx1OETQ8JKneopXpm3RG07AwMSMXw+lgo1d0DZiXwscJpcqHWm9eWaI7OQX6JFl96Yjnjjh1z8gLXFYMXDxLUB3wBtYF903ukiQ==").(*RRSIG),
			[]RR{
				dnstestNew("org.	30	IN	DNSKEY	256 3 8 AwEAAfCN1yguCELJYujNmis5cjZFEW4UcxwSitTh7m1RYMEHTjhSCCzeMZ+UrpNIxLDLwAtWCcAQPoLuQdbhbMBo17pkK2UF26k+WKLN4Ieyw8SsAFScQkf+K7wCYJ4fRJgpsvZgRYBwzpsfQjm26Jd8B7olV02AnvxUHuPo1QeGz0Dd"),
				dnstestNew("org.	30	IN	DNSKEY	256 3 8 AwEAAa0ig3MNvgtoYj85frPVdYmRm98PlHMHxQBQRRsGC4wucf5PHv9wlSQOJUfGXY45KlJpdgZX8B6s4vHZwOGW2utqp9u2JLMFaMb5bhZUlbD/qufIQu8hcLtoTkORdaK0lo+9vx8R+N2I0DcIVVWBjtSoNjb5L/ssPPicUsBobl2B"),
				dnstestNew("org.	30	IN	DNSKEY	257 3 8 AwEAAexZJ/1wfyNCxNPrTZizaG7UlibGhP+AyogR6bqjptKweEgE4gD8GxRQJkt+Fn5pCoNqzmm1ZnEoKqvm93uOYtbKkYQDGH+W69J66MSKpgIyS+mT/4iaXn+lpb5o99l/sf7lHMa975O/fqN6aPUll4hUbN2T1LHv6HzQuQCtNRJA8jHGwX5q0NMmh2Z+yaG6B9cISerje9l5L+ID2ydJ6zXquYteoIUvX2xzqnXCdHPSvD+oL6R/weW+tztdFS1hok/1z3tn5NzmcaOLll9nXniCozEpLFEGPswyvtphWgCYhI8bBTqhUsIwfIwLSBQTEg2oCX7sS5CbXg44OqwhIW8="),
			},
		},
		{
			"mldsa44-example.com",
			dnstestNew("example.com.	3600	IN	DNSKEY	257 3 18 17K0clSq4NtF55MNSpjSyX2PE5fReJ2voXAksxbpvslPyZRtQvGbeadBO7qjPnFJy0LtURVpOsBB+suYit61/g4dhjEYSZW1ksOX0ilOLhT5CqQUujgmiZrEP0zMrLwm6agyuVEY1ctDPL75ZgsAE44IF/YediyidMNq1VTrIqrBFi5KsBrLoeOMTv2PgLZbMz0PcuVd/nHOnB67mInnxWEGwP1zgDoq7P6v3teqPLLO2lTRK9jNNqeM+XWUO0er0l6ICsRS5XQu0ejRqCr6huWQx1jBWuTShA2SvKGlCQ9ASWWX/KfYuVE/GhvabpUKqpjeRnUH1KT1pPBZkhZYLDVy9i7aiQWrNYFnDEoCd3oz4Mpylf2PT/bRoKOnaD1l9fX3/GDaAj6CbF+SFEwC99G6EHWYdVPqk2f8122ZC3+pnNRa/biDbUPkWfUYffBYR5cJoB6mg1k1+nBGCZDNPcG6QBupS6sd3kGsZ6szGdysoGBI1MTu8n7hOpwX0FOPQw8tZC3CQVZg3niHfY2KvHJSOXjAQuQoX0MZhGxEEmJCl2hEwQ5Va6IVtacZ5Z0MayqW05hZBx/cws3nUkp77a5U6FsxjoVOj+Ky8+36yXGRKCcKr9HlBEw6T9r9n/MfkHhLjo5FlhRKDa9YZRHT2ZYrnqla8Ze05fxg8rHtFd46W+9fib3HnZEFHZsoFudPpUUx79wcvnTUSIV/R2vNWPIcC2U7O3ak4HamVZowJxhVXMY/dIWaq6uSXwI4YcqM0Pe62yhx9n1VMm10URNa1F9KG6aRGPuyyKMO7JOS7z+XcGbJrdXHEMxkexUU0hfZWMcBfD6Q/SDATmdLkEhuk3CjGgAdMvRzl55JBnSefkd/oLdFCPil8jeDErg8Jb04jKCw//dHi69CtxZn7arJfEaxKWQ+WG5bBVoMIRlG1PNuZ1vtWGD6BCoxXZgmFk1qkjfDWl+/SVSQpb1N8ki5XEqud4S2BWcxZqxCRbW0sIKgnpMj5i8geMW3Z4NEbe/XNq06NwLUmwiYRJAKYYMzl7xEGbMNepegs4fBkRR0xNQbU+Mql3rLbw6nXbZbs55Z5wHnaVfe9vLURVnDGncSK1IE47XCGfFoixTtC8C4AbPm6C3NQ+nA6fQXRM2YFb0byIINi7Ej8E+s0bG2hd1aKxuNu/PtkzZw8JWhgLTxktCLELj6u9/MKyRRjjLuoKXgyQTKhEeACD87DNLQuLavZ7w1W5SUAl3HsKePqA46Lb/rUTKIUdYHgZjpSTZRrnh+wCUfkiujDp9R32Km1yeEzz3SBTkxdt+jJKUSvZSXCjbdNKUUqGeR8Os28BRbCatkZRtKAxOymWEaKhxIiRYnWYdooxFAYLpEQ0ht9RUioc6IswmFwhb45u0XjdVnswSg1Mr7qIKig0LxepqiauWNtjAIPSw1j99WbD9dYqQoVnvJ6ozpXKoPNUdLC/qPM5olCrTfzyCDvo7vvBBV4Y/hU3DuyyYFZtg/8GshGq7EPKKbVMzQD4gVokZe8LRlFcx+QfMSTwnv/3OTCatYspoUWaALzlA46TjJZ49y6w5O5f2q5m2fhXP8l/xCtJWfS/i2HXhDPoawM11ukZHE2L9IezkFwQjP1qwksM633LfPUfhNDtaHuV6uscUzwG8NlwI9kqcIJYN7Wbpst9TlawqHwgOGKujzFbpZJejt76Z5NpoiAnZhUfFqll+fgeznbMBwtVhp5NuXhM8FyDCzJCyDEg==").(*DNSKEY),
			dnstestNew("example.com.	3600	IN	RRSIG	MX 18 2 3600 20150819220000 20150729220000 59829 example.com. kdySHzwB7NftjQSAF7snCeKau3NoqpLNg16h/eHZV8L3Zpi30lkRyiS4FLMMZqTjzbf1A/bShg4qZpYlnfqXN8uqFWF9GEEJOgte1CFdF4GC05gEBU88KryfnGAcpXKafw9htDxZrqmqVSWN+1guW7HyUUFo1IuWTnZKuhZptDJkq+Ml+5ZHy4p+2Tdwk8MH7tJlTYk/UVaM1wIXPB2YgJ++kD0zhys5c38rztcaOmMXt6ejyAEY37Dc1Z/KsrRQZWv+XZ/CTliuh+dGJHoGuTm5KwS0us884ukWNC/wIU/SdlGoBDVXsT163Tr6lTf8pJ4xixcKIN8nsKSFxP9j+AbaN5SofIAvp4LGIFLgMKsRV/cqeYo8PegVD2EhAQ2/HVTO3uO8vlqLK7nWVVK2+2aYKIL2EqzjhRYKU5DhMwS9ZgbG0niszGXpvZcNcOyABXysdVuaDjnUuamYVACOUrV786LNmt8IWDnXWoPPMErPk5vNyHq6+ZHg79UeZpSzx0Ae/1aIfi2WEta9Or5sGItBn6vFWi9kJRuhuoMIXf9CLBV/LHL/PIenBxXSnr2Owg54AuSN2tmk2lDy8BfKzzvxTOoKXx4edo96Xv6QWASAxO9JmyEvhnF3SBI6HG3fn2+k8rgJLIHpsr4pZhMh4/SQWaojxt51nEIFi1bl7P6sAmCdMP81LSNx05hIkKcPeO33hA2VSDO7GzOEsnBOzbhUX9gbFr3aNV/Wrbs/cZMAL1I0IKG20jkmEfZ9PeKN0hXCxHJo4hPFL2mm9ciGpuXS7oN8f7YublNTwRY8b4plScVICpyBT5UDOgezR9/+DnklL0fzIORMTRnpD1hq4BqZMgNMwvczFg3DrSLQP/cBiKLn3toJrkSuU9aXodEqW3lhRdMvDUqTtHgMKas5velmabpENAbixiB8n5zoENnMLV6w/13a+yOTT2WUvESgHqF92FfQMdQl36noyewmjUFZopirCGV6AkebdVsTY27DtYkGWamLXcm3w2d6AYV/LssvyK/Jlnw/E7YRJWkO+8PvHA2tvfQSr8fNC4ll/KHdwr8d0Q8spPcOHMMui20XDYeprPmp64hSt4IBuiQusdm3SQsWjQvaUsg8sykZd24S/wNQiGswXaoG6oWYYCZupfvGc0sgb+9qxZU5fSAYKwx5LjYajruvQ5flebAtrUdLuPbGMb2I7Z8c4IvDmbA6ljqMK60w1XI+wU7jSWzoEaiIeAUR1aT925KFMEhmFG3kTr5ZPI57wM7pEI9jBME80lu7D3f4z++icSHSJ5YNa/+kp7eSIT94m4Tj7nelmN0WnKFgzGZKnuiDGJew5FFnfB0qfvqUNUPt1rVaIr7rzBBL4j8WQHqOo17A+0pnIqKTe1Z8MxFnPwP1eWHa3T/7JeEPSD5JFOpEWxs12twxTC42BrTCckSmrfmksfxmJa0mfflaOPHkjahTprrItJzG1efHYCu5nP5rsclZF0hDOR1OZrgK2IhnG1VotIPB4+/+70+uD0qcqY3L2yonxFlQS8sEmMcXi9xQTxdFG4NOk/TQG50Oly1tRp9UoLjwTDtlIjh71Lz9lajbAabV4WtIvd7cwaREO0kFAtzIgfJRVMasWvUo6e93qQBThzvkCNs8ngsa0jXJL1HrERP+qkiULCDMr19FVimWmIzLCkR9pg9WWjruY5krgdVbINUqjsyyGriPEhy2JneNWdOdFoAwkWtGbIpQhHs2bLHpG9xPPF+ElqLmjNa76BhXv4caurHYn7K0m4NMVgDywGXoh0OGe/PoXQ4gHt7EbHgbCQO9V8+/1+MWw9ZrU6btOGJ2JVXeyRXYyJarn+cnPL1nWOlq7bMD3mazOTNZPc5UENSvDL51hmd3WD71i2u9btqIzjnmSxggPHRsVcOaGXHM3aUJnrDtwi1EY7THlJatS+ItjWQMCDh8g/4LF9S2UWGFc21MimswWvgh1jB/4hYI9C8PSCpAeV26dXoANntR/lLms42488dVJ1wyNGjaNNX1itiqFYsNUn3LyT3TdVUgBwkfzO1I4UnhDIbsHJWbs7Dl/52Ei4MbpPJXnL1gMNc6SD1EkT1CeY9fesHF20wr8tb7V+qPO2TCE26syB9lZ41OSOYgqPYK/OHyoLedQmTOFls0QMj2F0bks3pJm/TDDMEuUdhulPatnZBNIXexqNImQUFyipcJ9W5KnD6Wr5+jyULyVBQRpWPzipfPFACb5d5lWPtrvh4kurYt3sSdUy+WJKuYb1roxXTZJqP0QDgnVEYL5nJnxqSRD9fx7HMRHXODkVioBFmSUgwP5XBljn/YpIgG8Ix42hyKMCtiyv1gIY3/m8cfHyj5I6xcDHUTZHyM9+KSZeipf6wUnngoZuYzP9N3Nozo8LI+w3Mo6s/VjhmsALOYcus720s0MQY5prhkcZYUvgv9YL9R+1Fm7Kxy3cjpnGqyWwxN6YmNw/f6C+21Dlex7+09o2ygi0M1NEZZ0FhdaBmxVxtSjbBm3uKu9taW0zO534HXlifFkxf6GhboxbGdm1yekVIjDLnC+iodQyLwIi0vvc435Xk4GRBs8D5Pxf3vT3tgPy5sDXbJ3lT58MekKdT/HobugDOdu0ltGenFjnKFhdJudvQ/FFjqJk1HYnjxxdP3QYKlSHOv2ADtRqgI0VHLJmECOifYr90uWml1uzaUzK0XTulm8fn6lfpF3EWJYSsq1iXQWuiRw9u6dxiS02+c4Z8Nzumoh48W+z0GFy+qClyhqdedA6k3WZIJi919e5b24mj5rqzcgrA6KMqnTJDKh2cuoKC1fI88w774co0XPDyg+v/RD2ET1fquDGHjeVyVBsknNZQ5lwvLeAy/uH+Ql5qECQ9WCIJPydZZhB906hkHZ+vch1fG+vhgMtoXhtZ4UXzQwbJBL/4wxtOau3IgWGkJEImJPK3KE+7phfn5YmGSjVCp8o1t2QxpwJ1ZPBuTrUWy15gruIP8e415f0UPUZjFG+p6JqsUzaBzgZvAg9nY/vHEC0sXuC7lnqmDxr8LU9JMD77XrBccXMP199d/10bJW8TH+yzqE4syjdUPEalQnwP/fh9us92eSdv50vr0/KPhzfWzcRwWFxofS15zlJe3xNj+BAURHCApKjBkh5emuLy+w9zn6vn6/QsbXWp6hZWcoLO6ytLf6/H+DhguNzs/VFVbg5SXo62wztPoAAAAAAAAAAAAAA0jNEY=").(*RRSIG),
			[]RR{
				dnstestNew("example.com.	3600	IN	MX	10 mail.example.com."),
			},
		},
		{
			"mldsa44-mldsa.huque.com",
			dnstestNew("mldsa.huque.com.	7200	IN	DNSKEY	257 3 18 c0/mimzrnxEygW4YSsIj5kzNjg3hfxoutY5GyvxNtPugjVBQUoaIF2ji/5zhDOIo5L60khxlpotJDQ1yoNUBhF+dLPRejDCXleg58VKRoy30Qy35wU4vl3YXq+Y9auAYJrkCUdUl+7UBrneMMg7NP0Bx2zcO7IP1RBKUfSX9MDWcalojBL1yV4QFX8qGXmWFX+L1Z1aARX2KJUSleKo83d7YOUjL9VO1R85zAAO78gjpMcinusIlbn+6rhOHpyFfXSRlXvq53yOJCEQbQU+MBjqHBoV5bf7UW6p/RH4HEYRDwjq+/hYw7pdJeIR1oazYgJnLQLvb8rp0XzoMTbSAO3kqEwCKHMHnLXfYcsQv8E7mZT8G5Qat9Sr8HF5JiSNBh1qHn3e+fIU9kIzNnQU1gGL1OJcWITOIdRr3tdbJbzyfnAbQTdshr57b4zZjzyJP80GUwqEsOfy3M+bnaf8MfIDeeXpgHm04ZbQVXdgziUQmKiKr3KyUkJxPSQZ2cuPN/d6jh2FFpOsEWwTreYCJiTtC4++2mfVIma99wDZ6nb71kSItDNdkC1x1PHEyFlZIyMl1n3yuaeWo1+DqE+pW+LFd1JHCBpZcQiZGXOrh5WFjH41lwpcyEaRx50FH0uwAjmdhR92mTvXmKMjqSVxQu4yHVMmiTIPtRuyhBlHFvDbswE7MenJi1x+oeDBUWfNzPBvtgyjIT4Op2LEBtc6O04SMQZFZgogQtX7L0r8Ls+uW1aSo5uQqQs/RgUhcSCOevmGcE02H0/wsnyr05hcQBwjJm1FOyAGE1204FgwNXHQ+Zm+0vPAgxtNCFWL3brK6kdon8XI3DQWb83UANWPPP8u4642OqL93cn+pfopHoyL4unp/HTouBPFMTmirvYbNEjhnNrUoeRJLVniP6+lf3iVNVqbCKnRrVVdzm+mSlPXrsM/R/XYpxIhCncwccAujpLCiy9KQUU6e3SU1WlCdR5HFgGbpu1wN/Pp0/EKz530DUfvcaLnqWTQaeN1rGrrPfj1VE49VJxiQC5gp6R8KogG7QLhIiW4I216dGGrva09IRoy1K7i4lXAqVvur2aXrEYggkDOvuGhEkpIHxrszXhqR/bgcIx0/D8iTuwjt5Fi8bIxkdPTcF+Qg6e/OmaUOhCAJF6hv+xQeyHaKm4shjC7asTsNaZUyz76/NsZYffTQhBnU3nu3c3tDVicGx7pqdazBmIkWpCt1jnCCGzQMmrlShBiTzOarOHfmnz6PKXt1YEQZ7zBTMu/RMO0nnWO/crL/BfJBjLHylyNSg90nINg+v6ErhpI+AJasrtjQLdGXZ47jxsPr4yAkbeW1EOTdWA7W9hI5mJRJ41H9aem11zzc1FXKgQ8SPTmerbrVCFcI8zEf/w2r1KCzgzaVgX6EpPDgsLj4BUrj0kplWufJYdd/sABXfSK9TpFfBAAJUjbrJqIuDjwITdp4k4nwWvX5wmegdXaBvQG0lOi/iKUxgLqPtBSchYm+3fnO2BFBz+IqZbAyvE9ySMLO3KrMTwsEXjiB5zgSF/9IuNgQObh3IJ3T30NPPfhGGj8TECsSTV0V3iWd7YP39sP8h/N6HZT3NNJzyfT+WTkZFhI/VLHh6WWRWcdWmF4UtDHjBOICvm+VSSlaLJ7OuO6KK2992UUCEoRgtbAPZ7sUfv3IyWx6XZL3Cm7J0xMV124mIjttdH1yYBUwCIVyNm/QroPGnBeYVO4gJSNVWA9fJJj+9uGSrw==").(*DNSKEY),
			dnstestNew("mldsa.huque.com.	7200	IN	RRSIG	DNSKEY 18 3 7200 20260916193731 20260817171749 23583 mldsa.huque.com. 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").(*RRSIG),
			[]RR{
				dnstestNew("mldsa.huque.com.	7200	IN	DNSKEY	257 3 18 c0/mimzrnxEygW4YSsIj5kzNjg3hfxoutY5GyvxNtPugjVBQUoaIF2ji/5zhDOIo5L60khxlpotJDQ1yoNUBhF+dLPRejDCXleg58VKRoy30Qy35wU4vl3YXq+Y9auAYJrkCUdUl+7UBrneMMg7NP0Bx2zcO7IP1RBKUfSX9MDWcalojBL1yV4QFX8qGXmWFX+L1Z1aARX2KJUSleKo83d7YOUjL9VO1R85zAAO78gjpMcinusIlbn+6rhOHpyFfXSRlXvq53yOJCEQbQU+MBjqHBoV5bf7UW6p/RH4HEYRDwjq+/hYw7pdJeIR1oazYgJnLQLvb8rp0XzoMTbSAO3kqEwCKHMHnLXfYcsQv8E7mZT8G5Qat9Sr8HF5JiSNBh1qHn3e+fIU9kIzNnQU1gGL1OJcWITOIdRr3tdbJbzyfnAbQTdshr57b4zZjzyJP80GUwqEsOfy3M+bnaf8MfIDeeXpgHm04ZbQVXdgziUQmKiKr3KyUkJxPSQZ2cuPN/d6jh2FFpOsEWwTreYCJiTtC4++2mfVIma99wDZ6nb71kSItDNdkC1x1PHEyFlZIyMl1n3yuaeWo1+DqE+pW+LFd1JHCBpZcQiZGXOrh5WFjH41lwpcyEaRx50FH0uwAjmdhR92mTvXmKMjqSVxQu4yHVMmiTIPtRuyhBlHFvDbswE7MenJi1x+oeDBUWfNzPBvtgyjIT4Op2LEBtc6O04SMQZFZgogQtX7L0r8Ls+uW1aSo5uQqQs/RgUhcSCOevmGcE02H0/wsnyr05hcQBwjJm1FOyAGE1204FgwNXHQ+Zm+0vPAgxtNCFWL3brK6kdon8XI3DQWb83UANWPPP8u4642OqL93cn+pfopHoyL4unp/HTouBPFMTmirvYbNEjhnNrUoeRJLVniP6+lf3iVNVqbCKnRrVVdzm+mSlPXrsM/R/XYpxIhCncwccAujpLCiy9KQUU6e3SU1WlCdR5HFgGbpu1wN/Pp0/EKz530DUfvcaLnqWTQaeN1rGrrPfj1VE49VJxiQC5gp6R8KogG7QLhIiW4I216dGGrva09IRoy1K7i4lXAqVvur2aXrEYggkDOvuGhEkpIHxrszXhqR/bgcIx0/D8iTuwjt5Fi8bIxkdPTcF+Qg6e/OmaUOhCAJF6hv+xQeyHaKm4shjC7asTsNaZUyz76/NsZYffTQhBnU3nu3c3tDVicGx7pqdazBmIkWpCt1jnCCGzQMmrlShBiTzOarOHfmnz6PKXt1YEQZ7zBTMu/RMO0nnWO/crL/BfJBjLHylyNSg90nINg+v6ErhpI+AJasrtjQLdGXZ47jxsPr4yAkbeW1EOTdWA7W9hI5mJRJ41H9aem11zzc1FXKgQ8SPTmerbrVCFcI8zEf/w2r1KCzgzaVgX6EpPDgsLj4BUrj0kplWufJYdd/sABXfSK9TpFfBAAJUjbrJqIuDjwITdp4k4nwWvX5wmegdXaBvQG0lOi/iKUxgLqPtBSchYm+3fnO2BFBz+IqZbAyvE9ySMLO3KrMTwsEXjiB5zgSF/9IuNgQObh3IJ3T30NPPfhGGj8TECsSTV0V3iWd7YP39sP8h/N6HZT3NNJzyfT+WTkZFhI/VLHh6WWRWcdWmF4UtDHjBOICvm+VSSlaLJ7OuO6KK2992UUCEoRgtbAPZ7sUfv3IyWx6XZL3Cm7J0xMV124mIjttdH1yYBUwCIVyNm/QroPGnBeYVO4gJSNVWA9fJJj+9uGSrw=="),
			},
		},
		{
			"wildcard-multiple-rrs", // RFC 4035, 5.3.2: an answer expanded from *.miek.nl., signed elsewhere. The owner is reconstructed for every record, not just the first.
			dnstestNew("miek.nl. 3600 IN DNSKEY 256 3 13 CBXS0JBABBV9VF5SV6sQqklQf9MLxlnb3tFB6uq2ell2mJRoS+5qABIgrUTFrmCi7EQYmWK6CMMDwgIdIpZRXw==").(*DNSKEY),
			dnstestNew("www.miek.nl. 600 IN RRSIG A 13 2 600 21000101000000 20250101000000 64683 miek.nl. FEMuhN0OK1dJYKtkVe/uEkw/AM99hsvz3oexrZX0ESSlyVHEuCFDUWjtMEg6FbpJT3oSxCa6K06s3dmDZEJG8g==").(*RRSIG),
			[]RR{
				dnstestNew("www.miek.nl. 600 IN A 192.0.2.10"),
				dnstestNew("www.miek.nl. 600 IN A 192.0.2.11"),
			},
		},
	}

	options := &SignOption{}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sig.Verify(tc.key, tc.rrs, options)
			if err != nil {
				t.Fatalf("failure to verify: %s", err)
			}
		})
	}
}

func TestDNSSECPrivateKey(t *testing.T) {
	testcases := []struct {
		name    string
		privkey string
		err     bool
	}{
		{
			"MLDSA44", // mldsa44PrivateKey is the private key from section 6 of draft-westerbaan-dnssec-mldsa.
			`Private-key-format: v1.3
Algorithm: 18 (MLDSA44)
PrivateKey: AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=`,
			false,
		},
		{
			"ECDSA-should-fail",
			`Private-key-format: v1.3
Algorithm:`,
			true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var key *DNSKEY
			_, err := key.NewPrivate(tc.privkey)
			if err != nil && !tc.err {
				t.Fatalf("failure to read the private key: %s", err)
			}
		})
	}
}
