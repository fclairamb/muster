// Package orgprefix derives shortest-unique-prefix abbreviations for git org names.
package orgprefix

import "sort"

// Derive returns a map from each org in orgs to the shortest unique prefix
// that disambiguates it from every other org. Manual overrides are honored
// first; non-overridden orgs are then expanded to avoid colliding with
// overrides or with each other.
//
// Algorithm:
//  1. Apply overrides verbatim, marking those orgs as "fixed".
//  2. Each non-fixed org starts at length 1.
//  3. While any two non-fixed orgs share a prefix, OR a non-fixed prefix
//     equals a fixed prefix, expand all colliding non-fixed orgs by one.
//  4. Repeat until stable.
func Derive(orgs []string, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(orgs))
	if len(orgs) == 0 {
		return out
	}

	// Dedupe input.
	seen := make(map[string]struct{}, len(orgs))
	dedup := make([]string, 0, len(orgs))
	for _, o := range orgs {
		if _, ok := seen[o]; ok {
			continue
		}
		seen[o] = struct{}{}
		dedup = append(dedup, o)
	}
	sort.Strings(dedup)

	// Apply overrides; remember which orgs are fixed.
	fixed := make(map[string]bool, len(dedup))
	for _, o := range dedup {
		if v, ok := overrides[o]; ok && v != "" {
			out[o] = v
			fixed[o] = true
		}
	}

	// Initialize non-fixed orgs to prefix length 1 (or full name if shorter).
	lengths := make(map[string]int, len(dedup))
	for _, o := range dedup {
		if fixed[o] {
			continue
		}
		lengths[o] = 1
		if len(o) < 1 {
			lengths[o] = len(o)
		}
	}

	prefix := func(o string) string {
		n := lengths[o]
		if n > len(o) {
			n = len(o)
		}
		return o[:n]
	}

	// Iteratively resolve collisions.
	for {
		changed := false
		// Build current prefix → orgs map for non-fixed.
		groups := make(map[string][]string)
		for o := range lengths {
			groups[prefix(o)] = append(groups[prefix(o)], o)
		}
		// Collect fixed prefixes for collision detection.
		fixedPrefixes := make(map[string]bool, len(out))
		for _, p := range out {
			fixedPrefixes[p] = true
		}

		for p, members := range groups {
			collide := false
			if len(members) > 1 {
				collide = true
			}
			if fixedPrefixes[p] {
				collide = true
			}
			if !collide {
				continue
			}
			for _, m := range members {
				if lengths[m] < len(m) {
					lengths[m]++
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	for o := range lengths {
		out[o] = prefix(o)
	}
	return out
}
