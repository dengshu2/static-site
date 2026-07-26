package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const trashMetaFile = ".deploy-trash.json"

type TrashItem struct {
	ID        string    `json:"id"`
	Site      Site      `json:"site"`
	DeletedAt time.Time `json:"deletedAt"`
	Reason    string    `json:"reason"`
}

type Trash struct {
	dir       string
	retention time.Duration
}

func NewTrash(dataDir string, retention time.Duration) (*Trash, error) {
	t := &Trash{dir: filepath.Join(dataDir, "trash"), retention: retention}
	if err := os.MkdirAll(t.dir, 0o750); err != nil {
		return nil, err
	}
	return t, t.Cleanup()
}

func (t *Trash) Move(site Site, sourceDir, reason string) (TrashItem, error) {
	item := TrashItem{
		ID:        fmt.Sprintf("%s-%d-%s", site.Name, time.Now().UTC().Unix(), randSuffix()),
		Site:      site,
		DeletedAt: time.Now().UTC(),
		Reason:    reason,
	}
	dest := t.path(item.ID)
	if err := os.Rename(sourceDir, dest); err != nil {
		return TrashItem{}, err
	}
	if err := t.writeMeta(item); err != nil {
		_ = os.Rename(dest, sourceDir)
		return TrashItem{}, err
	}
	return item, nil
}

func (t *Trash) Restore(item TrashItem, dest string) error {
	source := t.path(item.ID)
	if _, err := os.Stat(dest); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(source, trashMetaFile)); err != nil {
		return err
	}
	if err := os.Rename(source, dest); err != nil {
		_ = t.writeMeta(item)
		return err
	}
	return nil
}

func (t *Trash) RollbackRestore(item TrashItem, source string) error {
	dest := t.path(item.ID)
	if err := os.Rename(source, dest); err != nil {
		return err
	}
	return t.writeMeta(item)
}

func (t *Trash) Get(id string) (TrashItem, error) {
	if !validTrashID(id) {
		return TrashItem{}, os.ErrNotExist
	}
	b, err := os.ReadFile(filepath.Join(t.path(id), trashMetaFile))
	if err != nil {
		return TrashItem{}, err
	}
	var item TrashItem
	if err := json.Unmarshal(b, &item); err != nil {
		return TrashItem{}, err
	}
	if item.ID != id {
		return TrashItem{}, errors.New("回收站元数据 ID 不匹配")
	}
	return item, nil
}

func (t *Trash) List() []TrashItem {
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		return nil
	}
	out := make([]TrashItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if item, err := t.Get(entry.Name()); err == nil {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeletedAt.After(out[j].DeletedAt) })
	return out
}

func (t *Trash) Usage() (count int, total int64) {
	for _, item := range t.List() {
		count++
		total += item.Site.Size
	}
	return
}

func (t *Trash) Purge(id string) error {
	if !validTrashID(id) {
		return os.ErrNotExist
	}
	if _, err := t.Get(id); err != nil {
		return err
	}
	return os.RemoveAll(t.path(id))
}

func (t *Trash) Cleanup() error {
	cutoff := time.Now().UTC().Add(-t.retention)
	for _, item := range t.List() {
		if item.DeletedAt.Before(cutoff) {
			if err := os.RemoveAll(t.path(item.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Trash) writeMeta(item TrashItem) error {
	b, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(t.path(item.ID), trashMetaFile), b, 0o640)
}

func (t *Trash) path(id string) string {
	return filepath.Join(t.dir, id)
}

var trashIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)

func validTrashID(id string) bool {
	return trashIDRe.MatchString(id) && filepath.Base(id) == id
}
