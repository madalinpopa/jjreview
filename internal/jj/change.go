package jj

import "encoding/json"

type Change struct {
	ChangeID    string
	CommitID    string
	Parents     []string
	Author      string
	Description string
	Immutable   bool
}

type SplitFile struct {
	Path    string
	Content string
	Mode    string
	Delete  bool
}

type splitFileJSON struct {
	Path    string
	Content []byte
	Mode    string
	Delete  bool
}

func (f SplitFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(splitFileJSON{Path: f.Path, Content: []byte(f.Content), Mode: f.Mode, Delete: f.Delete})
}

func (f *SplitFile) UnmarshalJSON(data []byte) error {
	var w splitFileJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*f = SplitFile{Path: w.Path, Content: string(w.Content), Mode: w.Mode, Delete: w.Delete}
	return nil
}
