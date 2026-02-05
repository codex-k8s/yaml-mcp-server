package maputil

import "sync"

// Pop removes key from map under lock and returns the previous value if present.
func Pop[K comparable, V any](mu *sync.Mutex, items map[K]V, key K) (V, bool) {
	mu.Lock()
	defer mu.Unlock()

	value, ok := items[key]
	if ok {
		delete(items, key)
	}
	return value, ok
}
