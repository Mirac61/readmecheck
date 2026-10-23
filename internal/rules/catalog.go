package rules

// RuleSummary is the display metadata for a rule, without its check function.
type RuleSummary struct {
	ID       string
	Title    string
	Category Category
	Severity Severity
	Weight   int
	Why      string
	Fix      string
}

// CategoryRules groups the rule catalog by category, in display order.
type CategoryRules struct {
	Category Category
	Label    string
	Rules    []RuleSummary
}

// Catalog returns the public rule metadata used by the UI. It is derived from
// the same ruleSet that the analyzer executes, so the landing page cannot drift
// from the actual checks.
func Catalog() []CategoryRules {
	byCategory := make(map[Category][]RuleSummary, len(categoryOrder))
	for _, rule := range ruleSet {
		byCategory[rule.Category] = append(byCategory[rule.Category], RuleSummary{
			ID:       rule.ID,
			Title:    rule.Title,
			Category: rule.Category,
			Severity: rule.Severity,
			Weight:   rule.Weight,
			Why:      rule.Why,
			Fix:      rule.Fix,
		})
	}

	catalog := make([]CategoryRules, 0, len(categoryOrder))
	for _, category := range categoryOrder {
		catalog = append(catalog, CategoryRules{
			Category: category,
			Label:    CategoryLabel(category),
			Rules:    byCategory[category],
		})
	}
	return catalog
}
