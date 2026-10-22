package rules

import (
	"math"
	"sort"
)

func scoreCategories(results []Result) []CategoryScore {
	scores := make([]CategoryScore, 0, len(categoryOrder))
	for _, category := range categoryOrder {
		score := CategoryScore{Category: category, Label: CategoryLabel(category)}
		for _, result := range results {
			if result.Rule.Category != category {
				continue
			}
			switch result.Status {
			case StatusPass:
				score.Passed++
				score.Earned += result.Rule.Weight
				score.TotalWeight += result.Rule.Weight
			case StatusFail:
				score.Failed++
				score.TotalWeight += result.Rule.Weight
			case StatusSkipped:
				score.Skipped++
			}
		}
		if score.TotalWeight > 0 {
			score.Applicable = true
			score.Score = percentage(score.Earned, score.TotalWeight)
		}
		scores = append(scores, score)
	}
	return scores
}

func overallScore(categories []CategoryScore) (int, string) {
	earned := 0
	total := 0
	for _, category := range categories {
		earned += category.Earned
		total += category.TotalWeight
	}
	if total == 0 {
		return 0, "F"
	}
	score := percentage(earned, total)
	return score, GradeFor(score)
}

func percentage(earned, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(earned) / float64(total) * 100))
}

func GradeFor(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func countResults(results []Result) Counts {
	var counts Counts
	for _, result := range results {
		switch result.Status {
		case StatusPass:
			counts.Passed++
		case StatusFail:
			counts.Failed++
		case StatusSkipped:
			counts.Skipped++
		}
	}
	return counts
}

func sortResults(results []Result) {
	categoryRank := make(map[Category]int, len(categoryOrder))
	for i, category := range categoryOrder {
		categoryRank[category] = i
	}
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if categoryRank[left.Rule.Category] != categoryRank[right.Rule.Category] {
			return categoryRank[left.Rule.Category] < categoryRank[right.Rule.Category]
		}
		if statusRank(left.Status) != statusRank(right.Status) {
			return statusRank(left.Status) < statusRank(right.Status)
		}
		if severityRank(left.EffectiveSeverity()) != severityRank(right.EffectiveSeverity()) {
			return severityRank(left.EffectiveSeverity()) < severityRank(right.EffectiveSeverity())
		}
		if left.Rule.Weight != right.Rule.Weight {
			return left.Rule.Weight > right.Rule.Weight
		}
		return left.Rule.ID < right.Rule.ID
	})
}

func statusRank(status Status) int {
	switch status {
	case StatusFail:
		return 0
	case StatusSkipped:
		return 1
	default:
		return 2
	}
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}
