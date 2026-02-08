package clickup

import (
	"sync"
	"time"

	"github.com/toba/bean-me-up/internal/beans"
)

// SyncStateProvider abstracts sync state storage for the syncer.
type SyncStateProvider interface {
	GetTaskID(beanID string) *string
	GetSyncedAt(beanID string) *time.Time
	SetTaskID(beanID, taskID string)
	SetSyncedAt(beanID string, t time.Time)
	Clear(beanID string)
	Flush() error
}

// extensionCache holds cached sync state for a single bean.
type extensionCache struct {
	taskID   string
	syncedAt *time.Time
}

// pendingOp represents a pending write operation.
type pendingOp struct {
	beanID string
	set    *beans.ExtensionDataOp // nil means remove
}

// ExtensionSyncProvider implements SyncStateProvider using beans' extension metadata.
type ExtensionSyncProvider struct {
	client *beans.Client
	mu     sync.RWMutex
	cache  map[string]*extensionCache
	ops    []pendingOp
}

// NewExtensionSyncProvider creates a provider pre-populated from a bean list.
func NewExtensionSyncProvider(client *beans.Client, beanList []beans.Bean) *ExtensionSyncProvider {
	p := &ExtensionSyncProvider{
		client: client,
		cache:  make(map[string]*extensionCache, len(beanList)),
	}

	for _, b := range beanList {
		taskID := b.GetExtensionString(beans.PluginClickUp, beans.ExtKeyTaskID)
		syncedAt := b.GetExtensionTime(beans.PluginClickUp, beans.ExtKeySyncedAt)

		if taskID != "" || syncedAt != nil {
			p.cache[b.ID] = &extensionCache{
				taskID:   taskID,
				syncedAt: syncedAt,
			}
		}
	}

	return p
}

func (p *ExtensionSyncProvider) GetTaskID(beanID string) *string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	c, ok := p.cache[beanID]
	if !ok || c.taskID == "" {
		return nil
	}
	return &c.taskID
}

func (p *ExtensionSyncProvider) GetSyncedAt(beanID string) *time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()

	c, ok := p.cache[beanID]
	if !ok {
		return nil
	}
	return c.syncedAt
}

func (p *ExtensionSyncProvider) SetTaskID(beanID, taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cache[beanID] == nil {
		p.cache[beanID] = &extensionCache{}
	}
	p.cache[beanID].taskID = taskID
	p.appendSetOp(beanID)
}

func (p *ExtensionSyncProvider) SetSyncedAt(beanID string, t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	utc := t.UTC()
	if p.cache[beanID] == nil {
		p.cache[beanID] = &extensionCache{}
	}
	p.cache[beanID].syncedAt = &utc
	p.appendSetOp(beanID)
}

func (p *ExtensionSyncProvider) Clear(beanID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.cache, beanID)
	p.ops = append(p.ops, pendingOp{beanID: beanID, set: nil})
}

// Flush writes all pending operations to beans via GraphQL.
// Set operations are batched; remove operations are executed individually.
func (p *ExtensionSyncProvider) Flush() error {
	p.mu.Lock()
	ops := p.ops
	p.ops = nil
	p.mu.Unlock()

	if len(ops) == 0 {
		return nil
	}

	// Deduplicate: keep only the last operation per bean ID
	seen := make(map[string]int, len(ops))
	for i, op := range ops {
		seen[op.beanID] = i
	}

	// Collect final set ops and remove ops
	var setOps []beans.ExtensionDataOp
	var removeIDs []string

	for beanID, idx := range seen {
		op := ops[idx]
		if op.set != nil {
			setOps = append(setOps, *op.set)
		} else {
			removeIDs = append(removeIDs, beanID)
		}
	}

	// Batch set operations
	if len(setOps) > 0 {
		if err := p.client.SetExtensionDataBatch(setOps); err != nil {
			return err
		}
	}

	// Remove operations individually
	for _, id := range removeIDs {
		if err := p.client.RemoveExtensionData(id, beans.PluginClickUp); err != nil {
			return err
		}
	}

	return nil
}

// appendSetOp adds or updates a pending set operation for the given bean.
// Must be called with p.mu held for writing.
func (p *ExtensionSyncProvider) appendSetOp(beanID string) {
	c := p.cache[beanID]
	data := map[string]any{
		beans.ExtKeyTaskID: c.taskID,
	}
	if c.syncedAt != nil {
		data[beans.ExtKeySyncedAt] = c.syncedAt.Format(time.RFC3339)
	}

	p.ops = append(p.ops, pendingOp{
		beanID: beanID,
		set: &beans.ExtensionDataOp{
			BeanID: beanID,
			Name:   beans.PluginClickUp,
			Data:   data,
		},
	})
}
