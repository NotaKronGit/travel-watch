package realroutes

import "encoding/json"

// RailAccessKey abstracts only the arrival station of a verified rail feeder.
// Flight chains and airport changes remain distinct, as does source evidence.
func RailAccessKey(c Candidate) string {
	s := c.Steps
	if len(s) < 5 || s[0].Mode != "transfer" || s[1].Mode != "train" || s[2].Mode != "transfer" || (s[3].Mode != "plane" && s[3].Mode != "flight") || s[len(s)-1].Mode != "transfer" {
		return ""
	}
	if s[0].From == "" || s[1].FromCode == "" || s[1].ToCode == "" || s[len(s)-1].To == "" {
		return ""
	}
	for i := 1; i < len(s); i++ {
		if s[i-1].To == "" || s[i-1].To != s[i].From {
			return ""
		}
	}
	key := []string{s[0].From, s[1].FromCode, s[len(s)-1].To, s[0].Evidence, s[1].Evidence, s[2].Evidence, s[len(s)-1].Evidence}
	for _, step := range s[3 : len(s)-1] {
		switch step.Mode {
		case "plane", "flight":
			if step.FromCode == "" || step.ToCode == "" {
				return ""
			}
			key = append(key, "plane", step.FromCode, step.ToCode, step.Evidence)
		case "transfer":
			key = append(key, "transfer", step.From, step.To, step.Evidence)
		default:
			return ""
		}
	}
	b, _ := json.Marshal(key)
	return string(b)
}
func topologyKey(steps []Step) string {
	parts := make([]string, 0, len(steps)*6)
	for _, s := range steps {
		parts = append(parts, s.FromCode, s.ToCode, s.From, s.To, s.Mode, s.Evidence)
	}
	b, _ := json.Marshal(parts)
	return string(b)
}
func cloneSteps(steps []Step) []Step {
	out := append([]Step(nil), steps...)
	for i := range out {
		out[i].ObservedDates = append([]string(nil), steps[i].ObservedDates...)
	}
	return out
}
