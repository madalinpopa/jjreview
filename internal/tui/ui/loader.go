package ui

type Loader struct {
	seq     int
	loading bool
	loaded  bool
	stale   bool
	err     error
}

func (l Loader) Due() bool {
	return !l.loading && (!l.loaded || l.stale)
}

func (l *Loader) Begin() int {
	l.seq++
	l.loading = true
	l.stale = false
	return l.seq
}

func (l *Loader) Finish(seq int, err error) bool {
	if seq != l.seq {
		return false
	}
	l.loading = false
	l.loaded = true
	l.err = err
	return true
}

func (l *Loader) MarkStale() {
	l.stale = true
}

func (l Loader) Loading() bool {
	return l.loading
}

func (l Loader) Loaded() bool {
	return l.loaded
}

func (l Loader) Stale() bool {
	return l.stale
}

func (l Loader) Err() error {
	return l.err
}
