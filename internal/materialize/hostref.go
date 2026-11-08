package materialize

func CurrentHostRef(refs []HostRef) (HostRef, bool) {
	live, order := liveHostState(refs)
	if len(order) == 0 {
		return HostRef{}, false
	}
	return live[order[len(order)-1]], true
}

func liveHostState(refs []HostRef) (map[string]HostRef, []string) {
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
	return live, order
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
