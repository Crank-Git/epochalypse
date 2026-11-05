package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

const (
	maxArmInputBytes  = 16 << 20
	maxPinsInputBytes = 1 << 20
)

type armOptions struct {
	stateDir, planPath, keyPath, bundlePath, pinsPath string
}

type armInputs struct {
	plan switchclient.Plan
	key  ed25519.PrivateKey
	pins map[string][32]byte
	id   string
}

func runSwitchArm(args []string, stdout, stderr io.Writer) int {
	options, err := parseArmOptions(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	inputs, err := loadArmInputs(options)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	db, err := installation.Open(options.stateDir, installation.Limits(window.DefaultLimits()))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = db.Close() }()
	store, err := switches.Attach(db)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	record, err := store.Get(inputs.id)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if !armRecordMatches(record, inputs) {
		_, _ = fmt.Fprintln(stderr, "arm plan does not match the retained draft")
		return 1
	}
	record, result, armErr, err := executeArm(store, record, inputs, options.bundlePath)
	if err != nil {
		if record.State == switches.StateUncertain || record.State == switches.StateArmed {
			if writeErr := json.NewEncoder(stdout).Encode(record); writeErr != nil {
				_, _ = fmt.Fprintln(stderr, writeErr)
				return 1
			}
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if result.Confirmed {
		if err := initializeCheckins(options.bundlePath, inputs, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := json.NewEncoder(stdout).Encode(record); err != nil {
		return 1
	}
	if armErr != nil {
		_, _ = fmt.Fprintln(stderr, armErr)
		return 1
	}
	if !result.Confirmed {
		_, _ = fmt.Fprintln(stderr, "arm remains uncertain")
		return 1
	}
	return 0
}

func initializeCheckins(bundle string, inputs armInputs, result switchclient.ArmResult) error {
	request, err := readArmEvidence(bundle+".arm", inputs.plan, inputs.key, result)
	if err != nil {
		return err
	}
	return switchclient.InitializeRenewal(bundle+".checkins", inputs.plan, inputs.key, request.Request, result.Armed)
}

func parseArmOptions(args []string, stderr io.Writer) (armOptions, error) {
	options := armOptions{}
	flags := flag.NewFlagSet("switch arm", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.stateDir, "state-dir", envDefault("EPOCHALYPSE_STATE_DIR", "var/state"), "Local state directory")
	flags.StringVar(&options.planPath, "plan", "", "Owner-only JSON plan file")
	flags.StringVar(&options.keyPath, "owner-key", "", "Owner-only base64 Ed25519 private key file")
	flags.StringVar(&options.bundlePath, "bundle", "", "Recovery bundle output path")
	flags.StringVar(&options.pinsPath, "pins", "", "JSON map of HTTPS endpoints to TLS SPKI SHA256 pins")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 || options.planPath == "" || options.keyPath == "" || options.bundlePath == "" || options.pinsPath == "" {
		return options, errors.New("provide --plan, --owner-key, --bundle, and --pins")
	}
	return options, nil
}

func loadArmInputs(options armOptions) (armInputs, error) {
	var inputs armInputs
	plan, err := readPrivatePlan(options.planPath)
	if err != nil {
		return inputs, err
	}
	key, err := readOwnerPrivateKey(options.keyPath)
	if err != nil {
		return inputs, err
	}
	pins, err := readEndpointPins(options.pinsPath)
	if err != nil {
		return inputs, err
	}
	prepareFrame, err := switchwire.DecodeFrame(plan.Prepare.Body)
	if err != nil {
		return inputs, errors.New("invalid switch prepare request")
	}
	inputs = armInputs{plan: plan, key: key, pins: pins, id: prepareFrame.State.Switch}
	if err := validateArmPlan(inputs.id, inputs.plan, inputs.key); err != nil {
		return armInputs{}, err
	}
	return inputs, nil
}

func armRecordMatches(record switches.Record, inputs armInputs) bool {
	validState := record.State == switches.StateDraft || record.State == switches.StateUncertain || record.State == switches.StateArmed
	return validState && planMatchesDraft(inputs.plan, inputs.key, record)
}

func executeArm(store *switches.Store, record switches.Record, inputs armInputs, bundlePath string) (switches.Record, switchclient.ArmResult, error, error) {
	if record.Arm != nil {
		previous, err := readArmEvidence(bundlePath+".arm", inputs.plan, inputs.key, switchclient.ArmResult{})
		if err != nil || switches.ValidateArmEvidence(record, previous) != nil {
			return record, switchclient.ArmResult{}, nil, errors.New("arm plan does not match retained owner intent")
		}
	}
	registered := false
	result, armErr := switchclient.PrepareAndArmWithIntent(context.Background(), inputs.plan, inputs.key, &switchclient.HTTPSReplica{Pins: inputs.pins}, bundlePath, func(request switchwire.Envelope) error {
		evidence := switches.ArmEvidence{OwnerKey: [32]byte(inputs.key.Public().(ed25519.PublicKey)), Certificate: inputs.plan.Certificate, CertificateSignature: inputs.plan.CertificateSignature, Request: request}
		if record.Arm != nil {
			evidence.Armed = append(evidence.Armed, record.Arm.Armed...)
		}
		updated, observeErr := store.ObserveArm(context.Background(), inputs.id, evidence)
		if observeErr != nil {
			return observeErr
		}
		record = updated
		registered = true
		return nil
	})
	evidence, evidenceErr := readArmEvidence(bundlePath+".arm", inputs.plan, inputs.key, result)
	if evidenceErr != nil {
		return record, result, armErr, evidenceErr
	}
	evidence.Armed = mergeArmReceipts(evidence.Armed, record.Arm)
	if registered {
		updated, observeErr := store.ObserveArm(context.Background(), inputs.id, evidence)
		if observeErr != nil {
			return record, result, armErr, errors.New("arm status remains uncertain; local evidence update failed")
		}
		record = updated
	}
	return record, result, armErr, nil
}

func mergeArmReceipts(receipts []switchwire.Envelope, prior *switches.ArmEvidence) []switchwire.Envelope {
	if prior == nil {
		return receipts
	}
	for _, receipt := range prior.Armed {
		found := false
		for _, current := range receipts {
			if bytes.Equal(current.Body, receipt.Body) && bytes.Equal(current.Signature, receipt.Signature) {
				found = true
				break
			}
		}
		if !found {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

func readPrivatePlan(path string) (switchclient.Plan, error) {
	data, err := readOwnerOnlyFile(path, maxArmInputBytes)
	if err != nil {
		return switchclient.Plan{}, fmt.Errorf("read private switch plan: %w", err)
	}
	var plan switchclient.Plan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return switchclient.Plan{}, errors.New("invalid private switch plan")
	}
	return plan, nil
}

func readOwnerPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := readOwnerOnlyFile(path, 4096)
	if err != nil {
		return nil, fmt.Errorf("read owner key: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid owner private key file")
	}
	return ed25519.PrivateKey(decoded), nil
}

func readEndpointPins(path string) (map[string][32]byte, error) {
	data, err := readBoundedFile(path, maxPinsInputBytes)
	if err != nil {
		return nil, fmt.Errorf("read TLS pins: %w", err)
	}
	var encoded map[string]string
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&encoded) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(encoded) == 0 {
		return nil, errors.New("invalid TLS pin map")
	}
	pins := make(map[string][32]byte, len(encoded))
	for endpoint, value := range encoded {
		pin, err := hex.DecodeString(value)
		if endpoint == "" || err != nil || len(pin) != 32 {
			return nil, errors.New("invalid TLS pin map")
		}
		var digest [32]byte
		copy(digest[:], pin)
		pins[endpoint] = digest
	}
	return pins, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	return readLimitedFile(path, limit, false)
}

func readLimitedFile(path string, limit int64, ownerOnly bool) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || ownerOnly && before.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("input must be an owner-only regular file")
	}
	file, err := os.Open(path) // #nosec G304 -- The owner supplies local input paths.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	after, pathErr := os.Lstat(path)
	if err != nil || pathErr != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || !os.SameFile(opened, after) || ownerOnly && opened.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("input must be a stable owner-only regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, errors.New("cannot read input file")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("input file exceeds limit")
	}
	return data, nil
}

func readOwnerOnlyFile(path string, limit int64) ([]byte, error) {
	return readLimitedFile(path, limit, true)
}

func validateArmPlan(id string, plan switchclient.Plan, key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid owner private key")
	}
	enrollment, err := switchwire.AdmitCertificate(plan.Certificate, plan.CertificateSignature, key.Public().(ed25519.PublicKey))
	if err != nil {
		return errors.New("invalid switch enrollment")
	}
	frame, err := switchwire.VerifyOwnerRequest(plan.Prepare, enrollment)
	if err != nil || frame.Operation != switchwire.OperationPrepare || frame.State.Switch != id || frame.State.Sequence != 1 {
		return errors.New("invalid switch prepare request")
	}
	if err := switchcontent.ValidateBinding(plan.Ciphertext, frame.State.Key, frame.State.Content); err != nil {
		return errors.New("switch content does not match the signed plan")
	}
	return nil
}

func planMatchesDraft(plan switchclient.Plan, key ed25519.PrivateKey, record switches.Record) bool {
	if switches.ValidateRecord(record) != nil {
		return false
	}
	frame, err := switchwire.DecodeFrame(plan.Prepare.Body)
	if err != nil {
		return false
	}
	enrollment, err := switchwire.AdmitCertificate(plan.Certificate, plan.CertificateSignature, key.Public().(ed25519.PublicKey))
	// #nosec G115 -- ValidateRecord bounds the configured duration before conversion.
	interval := uint64(record.IntervalSeconds)
	// #nosec G115 -- ValidateRecord bounds the configured duration before conversion.
	grace := uint64(record.GraceSeconds)
	return err == nil && enrollment.IntervalSeconds() == interval && frame.State.Switch == record.ID && frame.State.Grace == grace && frame.State.Deadline >= record.CreatedAt.Unix()+record.IntervalSeconds
}

func readArmEvidence(path string, plan switchclient.Plan, key ed25519.PrivateKey, result switchclient.ArmResult) (switches.ArmEvidence, error) {
	data, err := readOwnerOnlyFile(path, switchwire.MaxEnvelope+4)
	if err != nil {
		return switches.ArmEvidence{}, errors.New("signed arm request is unavailable")
	}
	request, err := switchwire.ReadEnvelope(bytes.NewReader(data))
	if err != nil {
		return switches.ArmEvidence{}, errors.New("signed arm request is invalid")
	}
	armed := make([]switchwire.Envelope, 0, len(result.Armed))
	ids := make([]string, 0, len(result.Armed))
	for id := range result.Armed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		armed = append(armed, result.Armed[id])
	}
	return switches.ArmEvidence{Certificate: plan.Certificate, CertificateSignature: plan.CertificateSignature, OwnerKey: [32]byte(key.Public().(ed25519.PublicKey)), Request: request, Armed: armed}, nil
}

func runSwitchExport(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("switch export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bundlePath := flags.String("bundle", "", "Recovery bundle to validate")
	keyPath := flags.String("owner-public-key", "", "Owner-only base64 Ed25519 public key file")
	outputPath := flags.String("output", "", "Validated recovery bundle output path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *bundlePath == "" || *keyPath == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "provide --bundle, --owner-public-key, and --output")
		return 2
	}
	keyData, err := readBoundedFile(*keyPath, 4096)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "read owner public key: invalid protected file")
		return 1
	}
	publicBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil || len(publicBytes) != ed25519.PublicKeySize {
		_, _ = fmt.Fprintln(stderr, "invalid owner public key file")
		return 1
	}
	file, err := os.Open(*bundlePath) // #nosec G304 -- The owner supplies local bundle paths.
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cannot open recovery bundle")
		return 1
	}
	bundle, readErr := switchclient.ReadBundle(file, ed25519.PublicKey(publicBytes))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := switchclient.ExportBundle(*outputPath, bundle, ed25519.PublicKey(publicBytes)); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "recovery bundle validated and exported")
	return 0
}
