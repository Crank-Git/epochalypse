module github.com/Crank-Git/epochalypse

go 1.27.3

replace codeberg.org/miekg/dns v0.6.128 => ./third_party/miekg-dns

require (
	codeberg.org/miekg/dns v0.6.128
	filippo.io/age v1.3.2
	github.com/drand/drand/v2 v2.1.8
	github.com/drand/kyber v1.3.3
	github.com/drand/tlock v1.2.0
	github.com/go-jose/go-jose/v4 v4.1.7
	go.etcd.io/bbolt v1.5.0
	golang.org/x/crypto v0.58.0
)

require (
	filippo.io/hpke v0.4.0 // indirect
	github.com/drand/kyber-bls12381 v0.3.5 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	go.dedis.ch/fixbuf v1.0.3 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
