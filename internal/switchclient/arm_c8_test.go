package switchclient

import (
	"context"
	"crypto/ed25519"
	"os"
	"testing"
)

func TestArmRejectsInvalidOwnerKeyBeforeTransmission(t *testing.T) {
	plan, _, replicas := armFixture(t)
	result, err := PrepareAndArm(context.Background(), plan, ed25519.PrivateKey{}, replicas, replicas.bundle)
	if err == nil || result.Confirmed || replicas.prepareCalls != 0 || replicas.armCalls != 0 {
		t.Fatalf("invalid owner key permits arm: result=%+v prepares=%d sends=%d error=%v", result, replicas.prepareCalls, replicas.armCalls, err)
	}
	if _, err := os.Stat(replicas.bundle + ".intent"); !os.IsNotExist(err) {
		t.Fatalf("invalid owner key writes durable intent: %v", err)
	}
}

func TestArmRejectsCiphertextThatDoesNotMatchSignedPlan(t *testing.T) {
	plan, key, replicas := armFixture(t)
	plan.Ciphertext[0] ^= 1
	result, err := PrepareAndArm(context.Background(), plan, key, replicas, replicas.bundle)
	if err == nil || result.Confirmed || replicas.prepareCalls != 0 || replicas.armCalls != 0 {
		t.Fatalf("changed ciphertext permits arm: result=%+v prepares=%d sends=%d error=%v", result, replicas.prepareCalls, replicas.armCalls, err)
	}
	if _, err := os.Stat(replicas.bundle + ".intent"); !os.IsNotExist(err) {
		t.Fatalf("changed ciphertext writes durable intent: %v", err)
	}
}
