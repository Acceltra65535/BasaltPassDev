package scope

import "testing"

func TestLegacyReadScopeDoesNotGrantManifestSubmission(t *testing.T) {
	if HasScope([]string{S2SRead}, S2SRBACManifestSubmit) {
		t.Fatal("legacy s2s.read must not grant the privileged manifest submission scope")
	}
	if !HasScope([]string{S2SRBACManifestSubmit}, S2SRBACManifestSubmit) {
		t.Fatal("dedicated manifest submission scope should be accepted")
	}
}
