package rules

import "testing"

func TestCatalogMatchesRuleSet(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != len(categoryOrder) {
		t.Fatalf("Catalog() returned %d categories, want %d", len(catalog), len(categoryOrder))
	}

	seen := make(map[string]bool, len(ruleSet))
	total := 0
	for _, group := range catalog {
		if len(group.Rules) == 0 {
			t.Errorf("category %q has no rules", group.Category)
		}
		for _, rule := range group.Rules {
			if seen[rule.ID] {
				t.Errorf("rule %q appears more than once", rule.ID)
			}
			seen[rule.ID] = true
			total++
		}
	}

	if total != len(ruleSet) {
		t.Fatalf("Catalog() returned %d rules, want %d", total, len(ruleSet))
	}
}
