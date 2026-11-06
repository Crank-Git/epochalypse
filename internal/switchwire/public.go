package switchwire

// PublicManifest retains authenticated enrollment and the agent's accepted release state.
type PublicManifest struct {
	Version              int       `json:"version"`
	Certificate          []byte    `json:"certificate"`
	CertificateSignature []byte    `json:"certificate_signature"`
	Accepted             Envelope  `json:"accepted"`
	Release              *Envelope `json:"release,omitempty"`
}
