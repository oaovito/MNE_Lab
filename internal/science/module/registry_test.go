package module

import "testing"

func TestScientificCapabilitiesRemainScoped(t *testing.T) {
	seen := map[string]Capability{}
	for _, c := range Capabilities() {
		if _, exists := seen[c.ID]; exists {
			t.Fatal("duplicate module", c.ID)
		}
		seen[c.ID] = c
	}
	for _, id := range []string{Zeta, NTA} {
		c := seen[id]
		if c.ScientificValidation != "PENDING" || len(c.Blockers) == 0 {
			t.Fatal("unsupported scientific validation", id)
		}
	}
	for _, id := range []string{UnknownMalvern, UnknownCompound} {
		if seen[id].ScientificValidation != "UNVALIDATED" {
			t.Fatal("container validity inherited scientific validation", id)
		}
	}
}
