package localepolicy

import (
	"strconv"
	"strings"
)

func base(tag string) string {
	return strings.ToLower(strings.SplitN(strings.TrimSpace(tag), "-", 2)[0])
}

// Resolve prioritizes the account, weighted browser preferences, then the
// deployment default. Unsupported or explicitly refused browser tags do not win.
func Resolve(account, header, fallback string, supported []string) string {
	allowed := map[string]bool{}
	for _, tag := range supported {
		switch base(tag) {
		case "zh", "en", "ar", "ko":
			allowed[base(tag)] = true
		}
	}
	fallback = base(fallback)
	if !allowed[fallback] {
		fallback = "en"
	}
	if allowed[base(account)] {
		return base(account)
	}
	selected, best := "", float64(-1)
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		tag, q := base(fields[0]), float64(1)
		valid := true
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(field), "=")
			if key != "q" {
				continue
			}
			var err error
			q, err = strconv.ParseFloat(value, 64)
			if !ok || err != nil || !(q >= 0 && q <= 1) {
				valid = false
			}
		}
		if tag == "*" {
			tag = fallback
		}
		if valid && allowed[tag] && q > 0 && q > best {
			selected, best = tag, q
		}
	}
	if selected != "" {
		return selected
	}
	return fallback
}
