package shortcuts

import (
	"errors"
	"fmt"
	"sort"
)

// Registry keeps OS registrations in sync. Call it on the owning UI thread.
// Register receives the command ID so adapters need not retain view objects.
type Registry struct {
	Active     map[string]string
	Register   func(key, commandID string) error
	Unregister func(key string)
}

// Apply changes only changed bindings. A strict apply rolls back on failure;
// startup uses best effort so one occupied key cannot disable all commands.
func (r *Registry) Apply(wanted map[string]string, strict bool) error {
	if r.Active == nil {
		r.Active = map[string]string{}
	}
	before := map[string]string{}
	for key, id := range r.Active {
		before[key] = id
	}
	for _, key := range sortedKeys(r.Active) {
		if wanted[key] != r.Active[key] {
			r.Unregister(key)
			delete(r.Active, key)
		}
	}
	var failures []error
	for _, key := range sortedKeys(wanted) {
		if r.Active[key] == wanted[key] {
			continue
		}
		if err := r.Register(key, wanted[key]); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", key, err))
			if strict {
				for _, added := range sortedKeys(r.Active) {
					if before[added] != r.Active[added] {
						r.Unregister(added)
						delete(r.Active, added)
					}
				}
				for _, removed := range sortedKeys(before) {
					if r.Active[removed] == before[removed] {
						continue
					}
					if restoreErr := r.Register(removed, before[removed]); restoreErr != nil {
						failures = append(failures, fmt.Errorf("restore %s: %w", removed, restoreErr))
					} else {
						r.Active[removed] = before[removed]
					}
				}
				return errors.Join(failures...)
			}
			continue
		}
		r.Active[key] = wanted[key]
	}
	return errors.Join(failures...)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
