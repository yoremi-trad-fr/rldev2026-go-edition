package rlsave

import "fmt"

type DiffEntry struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Index  int    `json:"index"`
	Offset int    `json:"offset"`
	Old    int64  `json:"old"`
	New    int64  `json:"new"`
}

type DiffSummary struct {
	Before  string      `json:"before,omitempty"`
	After   string      `json:"after,omitempty"`
	Kind    Kind        `json:"kind"`
	Label   string      `json:"label"`
	Changes []DiffEntry `json:"changes"`
}

func DiffSaves(before, after *Save) (DiffSummary, error) {
	if before == nil || after == nil {
		return DiffSummary{}, fmt.Errorf("nil save")
	}
	if before.Kind != after.Kind {
		return DiffSummary{}, fmt.Errorf("save kinds differ: %s vs %s", before.Kind, after.Kind)
	}
	if before.Container != after.Container {
		return DiffSummary{}, fmt.Errorf("save containers differ: %s vs %s", before.Container, after.Container)
	}

	changes, err := diffBodyDWords(before, after)
	if err != nil {
		return DiffSummary{}, err
	}
	if changes == nil {
		changes = []DiffEntry{}
	}
	return DiffSummary{
		Before:  before.Path,
		After:   after.Path,
		Kind:    before.Kind,
		Label:   before.Label,
		Changes: changes,
	}, nil
}

func diffBodyDWords(before, after *Save) ([]DiffEntry, error) {
	switch before.Kind {
	case KindGlobal:
		return diffGlobalInts(before, after)
	case KindRead:
		return diffDWords(before, after, "seen", "seen[%d]", false), nil
	case KindSystem:
		return diffDWords(before, after, "system_dword", "dword[%d]", false), nil
	default:
		return diffDWords(before, after, "body_dword", "dword[%d]", false), nil
	}
}

func diffGlobalInts(before, after *Save) ([]DiffEntry, error) {
	if len(before.Body) < GlobalIntCount*4 {
		return nil, fmt.Errorf("before global body too small for intG table: %d bytes", len(before.Body))
	}
	if len(after.Body) < GlobalIntCount*4 {
		return nil, fmt.Errorf("after global body too small for intG table: %d bytes", len(after.Body))
	}
	var changes []DiffEntry
	for i := 0; i < GlobalIntCount; i++ {
		off := i * 4
		oldValue := int64(int32(le32(before.Body, off)))
		newValue := int64(int32(le32(after.Body, off)))
		if oldValue == newValue {
			continue
		}
		changes = append(changes, DiffEntry{
			Name:   fmt.Sprintf("intG[%d]", i),
			Kind:   "intG",
			Index:  i,
			Offset: off,
			Old:    oldValue,
			New:    newValue,
		})
	}
	changes = appendBodySizeChange(changes, before, after)
	return changes, nil
}

func diffDWords(before, after *Save, kind, nameFormat string, signed bool) []DiffEntry {
	count := len(before.Body) / 4
	if afterCount := len(after.Body) / 4; afterCount < count {
		count = afterCount
	}
	var changes []DiffEntry
	for i := 0; i < count; i++ {
		off := i * 4
		oldRaw := le32(before.Body, off)
		newRaw := le32(after.Body, off)
		if oldRaw == newRaw {
			continue
		}
		oldValue := int64(oldRaw)
		newValue := int64(newRaw)
		if signed {
			oldValue = int64(int32(oldRaw))
			newValue = int64(int32(newRaw))
		}
		changes = append(changes, DiffEntry{
			Name:   fmt.Sprintf(nameFormat, i),
			Kind:   kind,
			Index:  i,
			Offset: off,
			Old:    oldValue,
			New:    newValue,
		})
	}
	changes = appendBodySizeChange(changes, before, after)
	return changes
}

func appendBodySizeChange(changes []DiffEntry, before, after *Save) []DiffEntry {
	if len(before.Body) == len(after.Body) {
		return changes
	}
	return append(changes, DiffEntry{
		Name:   "body_size",
		Kind:   "body_size",
		Index:  -1,
		Offset: -1,
		Old:    int64(len(before.Body)),
		New:    int64(len(after.Body)),
	})
}
