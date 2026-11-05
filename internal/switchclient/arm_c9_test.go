package switchclient

import (
	"context"
	"os"
	"testing"
)

func TestArmRejectsUnavailableReplicaBeforeWritingIntent(t *testing.T) {
	plan, key, replicas := armFixture(t)
	result, err := PrepareAndArm(context.Background(), plan, key, nil, replicas.bundle)
	if err == nil || result.Confirmed || len(result.Prepared) != 0 || len(result.Armed) != 0 {
		t.Fatalf("missing replica permits arm: result=%+v error=%v", result, err)
	}
	if replicas.prepareCalls != 0 || replicas.armCalls != 0 {
		t.Fatalf("missing replica sends requests: prepares=%d arms=%d", replicas.prepareCalls, replicas.armCalls)
	}
	if _, err := os.Stat(replicas.bundle + ".intent"); !os.IsNotExist(err) {
		t.Fatalf("missing replica writes durable intent: %v", err)
	}
}

func TestArmRejectsInvalidEnrollmentBeforeTransmission(t *testing.T) {
	plan, key, replicas := armFixture(t)
	plan.CertificateSignature[0] ^= 1
	result, err := PrepareAndArm(context.Background(), plan, key, replicas, replicas.bundle)
	if err == nil || result.Confirmed || replicas.prepareCalls != 0 || replicas.armCalls != 0 {
		t.Fatalf("invalid enrollment permits arm: result=%+v prepares=%d sends=%d error=%v", result, replicas.prepareCalls, replicas.armCalls, err)
	}
	if _, err := os.Stat(replicas.bundle + ".intent"); !os.IsNotExist(err) {
		t.Fatalf("invalid enrollment writes durable intent: %v", err)
	}
}
