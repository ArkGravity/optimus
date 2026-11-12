//go:build dbtest

package rbac

import "time"

// ExpireUserForTest controls TTL state without changing the production API or
// depending on database latency and wall-clock sleeps in integration tests.
func (p *PermissionCache) ExpireUserForTest(userID uint64) bool {
	value, ok := p.m.Load(userID)
	if !ok {
		return false
	}
	entry := value.(cacheEntry)
	entry.stored = time.Now().Add(-p.ttl - time.Second)
	p.m.Store(userID, entry)
	return true
}
