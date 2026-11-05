package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/agent"
)

// agentRuntimeSettings contains local operator pins and private agent keys.
type agentRuntimeSettings struct {
	Authority            agent.Config       `json:"authority"`
	AgentID              string             `json:"agent_id"`
	OwnerKey             ed25519.PublicKey  `json:"owner_key"`
	Certificate          []byte             `json:"certificate"`
	CertificateSignature []byte             `json:"certificate_signature"`
	SigningKey           ed25519.PrivateKey `json:"signing_key"`
	AgeIdentity          string             `json:"age_identity"`
}

func runAgentRuntime(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "", "Protected agent runtime configuration")
	lab := flags.Bool("lab-only-synthetic-clock", false, "Use a synthetic clock only for local experiments")
	address := flags.String("http-addr", "127.0.0.1:8443", "HTTPS listen address")
	cert := flags.String("tls-cert", "", "TLS certificate file")
	key := flags.String("tls-key", "", "TLS private key file")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *path == "" {
		_, _ = fmt.Fprintln(stderr, "agent runtime configuration is required")
		return 2
	}
	if args[1] == "serve" && !*lab {
		_, _ = fmt.Fprintln(stderr, "explicit lab clock mode is required; production clock evidence is unavailable")
		return 2
	}
	settings, err := readAgentRuntimeSettings(*path)
	if err == nil {
		if args[1] == "initialize" {
			err = agent.Initialize(settings.Authority)
		} else {
			err = serveAgentRuntime(ctx, settings, *address, *cert, *key)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if args[1] == "initialize" {
		if _, err := fmt.Fprintln(stdout, "agent authority initialized"); err != nil {
			return 1
		}
	}
	return 0
}

func readAgentRuntimeSettings(path string) (agentRuntimeSettings, error) {
	data, err := readOwnerOnlyFile(path, 64<<10)
	if err != nil {
		return agentRuntimeSettings{}, fmt.Errorf("read protected agent configuration: %w", err)
	}
	defer clear(data)
	var settings agentRuntimeSettings
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return agentRuntimeSettings{}, errors.New("agent runtime configuration is invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return agentRuntimeSettings{}, errors.New("agent runtime configuration has trailing data")
	}
	return settings, nil
}

func serveAgentRuntime(ctx context.Context, settings agentRuntimeSettings, address, certPath, keyPath string) error {
	certificate, err := readAgentTLSIdentity(certPath, keyPath)
	if err != nil {
		return errors.New("load agent HTTPS identity")
	}
	runtime, err := agent.OpenService(agent.ServiceConfig{
		Authority: settings.Authority, LabOnlySyntheticClock: true,
		Transition: agent.TransitionConfig{
			OwnerKey: settings.OwnerKey, EnrollmentBody: settings.Certificate,
			EnrollmentSignature: settings.CertificateSignature, AgentID: settings.AgentID,
			SigningKey: settings.SigningKey, AgeIdentity: settings.AgeIdentity,
			ReadClock: agent.NewLabClock(time.Now),
		},
	})
	if err != nil {
		return fmt.Errorf("open agent runtime: %w", err)
	}
	defer func() { _ = runtime.Close() }()
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("bind agent HTTPS address: %w", err)
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{
		Handler: runtime.Handler(), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}},
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384,
	}
	defer func() { _ = server.Close() }()
	result := make(chan error, 1)
	go func() { result <- server.ServeTLS(listener, "", "") }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve agent HTTPS: %w", err)
	}
}

func readAgentTLSIdentity(certPath, keyPath string) (tls.Certificate, error) {
	key, err := readOwnerOnlyFile(keyPath, 64<<10)
	if err != nil {
		return tls.Certificate{}, err
	}
	defer clear(key)
	cert, err := readBoundedFile(certPath, 64<<10)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(cert, key)
}
