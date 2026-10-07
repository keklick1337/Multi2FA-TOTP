package vault

import (
	"errors"
	"slices"
	"strings"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
)

// Folders returns a snapshot sorted by name.
func (v *Vault) Folders() []*Folder {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := slices.Clone(v.folders)
	slices.SortFunc(out, func(a, b *Folder) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

func (v *Vault) Folder(id string) *Folder {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.folderLocked(id)
}

func (v *Vault) folderLocked(id string) *Folder {
	if id == "" {
		return nil
	}
	for _, f := range v.folders {
		if f.ID == id {
			return f
		}
	}
	return nil
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "/", "-"))
	if name == "" {
		return "", errors.New("folder name is empty")
	}
	return name, nil
}

// AddFolder creates a folder under parent ("" for the top level).
func (v *Vault) AddFolder(name, parent string) (*Folder, error) {
	name, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if parent != "" && v.folderLocked(parent) == nil {
		return nil, ErrFolderNotFound
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	f := &Folder{ID: id, Name: name, Parent: parent, Created: nowUTC()}
	v.folders = append(v.folders, f)
	if err := v.saveLocked(); err != nil {
		v.folders = v.folders[:len(v.folders)-1]
		return nil, err
	}
	return f, nil
}

func (v *Vault) RenameFolder(id, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	f := v.folderLocked(id)
	if f == nil {
		return ErrFolderNotFound
	}
	old := f.Name
	f.Name = name
	if err := v.saveLocked(); err != nil {
		f.Name = old
		return err
	}
	return nil
}

// descendantsLocked returns id and every folder below it.
func (v *Vault) descendantsLocked(id string) map[string]bool {
	set := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, f := range v.folders {
			if !set[f.ID] && set[f.Parent] {
				set[f.ID] = true
				changed = true
			}
		}
	}
	return set
}

// Subtree returns the IDs of a folder and all folders nested in it.
func (v *Vault) Subtree(id string) map[string]bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.descendantsLocked(id)
}

// MoveFolder re-parents a folder; moving it into itself or a descendant is refused.
func (v *Vault) MoveFolder(id, parent string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	f := v.folderLocked(id)
	if f == nil {
		return ErrFolderNotFound
	}
	if parent != "" {
		if v.folderLocked(parent) == nil {
			return ErrFolderNotFound
		}
		if v.descendantsLocked(id)[parent] {
			return ErrFolderCycle
		}
	}
	old := f.Parent
	f.Parent = parent
	if err := v.saveLocked(); err != nil {
		f.Parent = old
		return err
	}
	return nil
}

// DeleteFolder removes a folder; its subfolders and entries move up to its parent.
func (v *Vault) DeleteFolder(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	f := v.folderLocked(id)
	if f == nil {
		return ErrFolderNotFound
	}
	oldFolders := slices.Clone(v.folders)
	type move struct {
		e    *Entry
		from string
	}
	var moved []move
	var reparented []*Folder
	for _, c := range v.folders {
		if c.Parent == id {
			c.Parent = f.Parent
			reparented = append(reparented, c)
		}
	}
	for _, e := range v.entries {
		if e.Folder == id {
			moved = append(moved, move{e, e.Folder})
			e.Folder = f.Parent
		}
	}
	v.folders = slices.DeleteFunc(v.folders, func(x *Folder) bool { return x.ID == id })
	if err := v.saveLocked(); err != nil {
		v.folders = oldFolders
		for _, c := range reparented {
			c.Parent = id
		}
		for _, m := range moved {
			m.e.Folder = m.from
		}
		return err
	}
	return nil
}

// MoveEntries puts entries into a folder ("" for the top level).
func (v *Vault) MoveEntries(ids []string, folder string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if folder != "" && v.folderLocked(folder) == nil {
		return ErrFolderNotFound
	}
	prev := map[*Entry]string{}
	for _, e := range v.entries {
		if slices.Contains(ids, e.ID) {
			prev[e] = e.Folder
			e.Folder = folder
		}
	}
	if err := v.saveLocked(); err != nil {
		for e, f := range prev {
			e.Folder = f
		}
		return err
	}
	return nil
}

// FolderPath returns folder names from the top level down to id.
func (v *Vault) FolderPath(id string) []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.pathLocked(id)
}

func (v *Vault) pathLocked(id string) []string {
	var names []string
	for seen := 0; id != "" && seen <= len(v.folders); seen++ {
		f := v.folderLocked(id)
		if f == nil {
			break
		}
		names = append([]string{f.Name}, names...)
		id = f.Parent
	}
	return names
}

// ensurePathLocked finds or creates nested folders by name and returns the deepest folder ID.
func (v *Vault) ensurePathLocked(names []string) (string, error) {
	parent := ""
	for _, name := range names {
		var found *Folder
		for _, f := range v.folders {
			if f.Parent == parent && strings.EqualFold(f.Name, name) {
				found = f
				break
			}
		}
		if found == nil {
			id, err := newID()
			if err != nil {
				return "", err
			}
			found = &Folder{ID: id, Name: name, Parent: parent, Created: nowUTC()}
			v.folders = append(v.folders, found)
		}
		parent = found.ID
	}
	return parent, nil
}

// ImportFrom copies entries of another open vault, recreating their folder paths by name.
// With allFolders the whole folder tree of src is recreated, including empty folders.
// Entries whose secret already exists here are skipped.
func (v *Vault) ImportFrom(src *Vault, entries []*Entry, allFolders bool) (added, skipped int, err error) {
	if src == v {
		return 0, 0, errors.New("source and target vault are the same")
	}
	src.mu.Lock()
	paths := make([][]string, len(entries))
	for i, e := range entries {
		paths[i] = src.pathLocked(e.Folder)
	}
	var tree [][]string
	if allFolders {
		for _, f := range src.folders {
			tree = append(tree, src.pathLocked(f.ID))
		}
	}
	src.mu.Unlock()

	v.mu.Lock()
	defer v.mu.Unlock()
	oldEntries, oldFolders := slices.Clone(v.entries), slices.Clone(v.folders)
	rollback := func() { v.entries, v.folders = oldEntries, oldFolders }
	for _, p := range tree {
		if _, err := v.ensurePathLocked(p); err != nil {
			rollback()
			return 0, 0, err
		}
	}
	for i, e := range entries {
		raw, err := e.box.open(e.sealed)
		if err != nil {
			rollback()
			return 0, 0, err
		}
		fp, err := v.box.fingerprint(raw.Bytes())
		if err != nil || v.hasFPLocked(fp, "") {
			raw.Destroy()
			if err != nil {
				rollback()
				return 0, 0, err
			}
			skipped++
			continue
		}
		c := &Entry{Key: e.Key, Notes: e.Notes, Tags: slices.Clone(e.Tags), Favorite: e.Favorite, Created: e.Created}
		c.Secret = ""
		err = v.attachSecretLocked(c, raw.Bytes())
		raw.Destroy()
		if err != nil {
			rollback()
			return 0, 0, err
		}
		if c.ID, err = newID(); err != nil {
			rollback()
			return 0, 0, err
		}
		if c.Folder, err = v.ensurePathLocked(paths[i]); err != nil {
			rollback()
			return 0, 0, err
		}
		if c.Created.IsZero() {
			c.Created = nowUTC()
		}
		c.Updated = nowUTC()
		v.entries = append(v.entries, c)
		added++
	}
	if added == 0 && len(v.folders) == len(oldFolders) {
		return 0, skipped, nil
	}
	if err := v.saveLocked(); err != nil {
		rollback()
		return 0, 0, err
	}
	return added, skipped, nil
}

// AddKeys imports scanned keys into a folder, skipping invalid ones and duplicates.
func (v *Vault) AddKeys(keys []*otp.Key, folder string) (added, skipped int, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.folderLocked(folder) == nil {
		folder = ""
	}
	old := slices.Clone(v.entries)
	for _, k := range keys {
		kk := *k
		if kk.Normalize() != nil {
			skipped++
			continue
		}
		raw, derr := otp.DecodeSecret(kk.Secret)
		if derr != nil {
			skipped++
			continue
		}
		fp, ferr := v.box.fingerprint(raw)
		if ferr != nil || v.hasFPLocked(fp, "") {
			wipe(raw)
			skipped++
			continue
		}
		e := &Entry{Key: kk, Folder: folder, Created: nowUTC(), Updated: nowUTC()}
		e.ID, err = newID()
		if err == nil {
			err = v.attachSecretLocked(e, raw)
		}
		wipe(raw)
		if err != nil {
			v.entries = old
			return 0, 0, err
		}
		v.entries = append(v.entries, e)
		added++
	}
	if added == 0 {
		return 0, skipped, nil
	}
	if err := v.saveLocked(); err != nil {
		v.entries = old
		return 0, 0, err
	}
	return added, skipped, nil
}
