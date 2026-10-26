package repository

import "testing"

func TestHashTrialParametersIsStable(t *testing.T) {
	parameters := map[string]any{"max_positions": 10, "rank_buffer": 15}
	firstHash, firstJSON, err := HashTrialParameters(parameters)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, secondJSON, err := HashTrialParameters(parameters)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || string(firstJSON) != string(secondJSON) || len(firstHash) != 64 {
		t.Fatalf("trial parameter hash is not stable: %q %q", firstHash, secondHash)
	}
}

func TestValidDatasetRole(t *testing.T) {
	for _, role := range []string{"development", "walk_forward", "holdout", "forward"} {
		if !validDatasetRole(role) {
			t.Errorf("role %q rejected", role)
		}
	}
	if validDatasetRole("unknown") {
		t.Fatal("unsupported dataset role accepted")
	}
}
