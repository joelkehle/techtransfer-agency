package operator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type PersistedState struct {
	Cursor      int                    `json:"cursor"`
	Submissions map[string]*Submission `json:"submissions"`
}

func LoadState(path string) (PersistedState, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PersistedState{Submissions: map[string]*Submission{}}, nil
		}
		return PersistedState{}, err
	}
	var state PersistedState
	if err := json.Unmarshal(blob, &state); err != nil {
		return PersistedState{}, err
	}
	if state.Submissions == nil {
		state.Submissions = map[string]*Submission{}
	}
	return state, nil
}

func SaveState(path string, state PersistedState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
