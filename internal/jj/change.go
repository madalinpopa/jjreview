package jj

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

type Change struct {
	ChangeID    string
	CommitID    string
	Parents     []string
	Author      string
	Description string
	Immutable   bool
	WorkingCopy bool
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

func (f SplitFile) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, splitFileJSON{Path: f.Path, Content: []byte(f.Content), Mode: f.Mode, Delete: f.Delete})
}

func (f *SplitFile) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var w splitFileJSON
	if err := json.UnmarshalDecode(dec, &w); err != nil {
		return err
	}
	*f = SplitFile{Path: w.Path, Content: string(w.Content), Mode: w.Mode, Delete: w.Delete}
	return nil
}
