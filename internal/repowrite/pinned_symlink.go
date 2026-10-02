package repowrite

// Symlink creates the link itself inside the pinned root. Its target is never
// traversed by a write; subsequent writes through an escaping link are refused.
func (r *PinnedRoot) Symlink(target, relative string) error {
	if _, err := Relative(relative); err != nil {
		return err
	}
	return r.root.Symlink(target, relative)
}
