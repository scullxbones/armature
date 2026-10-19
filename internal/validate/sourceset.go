package validate

type SourceSet struct {
	ids    map[string]struct{}
	err    error
	loaded bool
}

func KnownSources(ids map[string]struct{}) SourceSet {
	if ids == nil {
		ids = map[string]struct{}{}
	}
	return SourceSet{ids: ids, loaded: true}
}

func UnreadableSources(err error) SourceSet {
	return SourceSet{err: err, loaded: true}
}

func (s SourceSet) Loaded() bool {
	return s.loaded
}

func (s SourceSet) ParseError() error {
	return s.err
}

func (s SourceSet) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}
