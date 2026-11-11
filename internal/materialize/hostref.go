package materialize

func CurrentHostRef(refs []HostRef) (HostRef, bool) {
	live := LiveHostRefs(refs)
	if len(live) == 0 {
		return HostRef{}, false
	}
	return live[len(live)-1], true
}

// LiveHostRefs returns the live host names in the order they were recorded.
// A later record of the same URL replaces the earlier one and moves it to the
// end. A withdrawn URL drops out until a later record adds it again.
func LiveHostRefs(refs []HostRef) []HostRef {
	live := map[string]HostRef{}
	order := []string{}
	for _, ref := range refs {
		if ref.Withdrawn {
			delete(live, ref.URL)
			order = dropURL(order, ref.URL)
			continue
		}
		order = dropURL(order, ref.URL)
		order = append(order, ref.URL)
		live[ref.URL] = ref
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]HostRef, 0, len(order))
	for _, url := range order {
		out = append(out, live[url])
	}
	return out
}

func dropURL(order []string, url string) []string {
	next := make([]string, 0, len(order))
	for _, item := range order {
		if item != url {
			next = append(next, item)
		}
	}
	return next
}
