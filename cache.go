package lrutree

import (
	"container/list"
	"errors"
	"sync"
)

var (
	ErrRootAlreadyExists = errors.New("root node already exists")
	ErrParentNotExist    = errors.New("parent node does not exist")
	ErrAlreadyExists     = errors.New("node already exists")
	ErrCycleDetected     = errors.New("cycle detected")
)

// StatsCollector is an interface for collecting cache metrics and statistics.
type StatsCollector interface {
	// SetAmount sets the total number of entries in the cache.
	SetAmount(int)

	// IncHits increments the total number of successfully found keys in the cache.
	IncHits()

	// IncMisses increments the total number of not found keys in the cache.
	IncMisses()

	// AddEvictions increments the total number of evicted entries.
	AddEvictions(int)
}

// Cache is a hierarchical cache with LRU (Least Recently Used) eviction policy.
//
// It maintains parent-child relationships between nodes in a tree structure while
// ensuring that parent nodes remain in cache as long as their children exist.
//
// Key properties:
//   - Each node has a unique key and is identified by that key.
//   - Nodes form a tree structure with parent-child relationships.
//   - Each node (except root) has exactly one parent and possibly multiple children.
//   - When a node is accessed, both it and all its ancestors are marked as recently used.
//   - When eviction occurs, the least recently used node is removed. It guarantees the evicted node is a leaf.
//   - If a node is present in the cache, all its ancestors up to the root are guaranteed to be present.
//
// This cache is particularly useful for hierarchical data where accessing a child
// implies that its ancestors are also valuable and should remain in cache.
type Cache[K comparable, V any] struct {
	maxEntries int
	onEvict    func(node CacheNode[K, V])
	stats      StatsCollector
	mu         sync.RWMutex
	keysMap    map[K]*treeNode[K, V]
	lruList    *list.List
	root       *treeNode[K, V]
}

// CacheNode represents a node in the cache with its key, value, and parent key.
// It is used to return node information from the Cache methods.
type CacheNode[K comparable, V any] struct {
	Key       K
	Value     V
	ParentKey K
}

type treeNode[K comparable, V any] struct {
	key      K
	val      V
	parent   *treeNode[K, V]
	children map[K]*treeNode[K, V]
	lruElem  *list.Element
}

func newTreeNode[K comparable, V any](key K, val V, parent *treeNode[K, V]) *treeNode[K, V] {
	return &treeNode[K, V]{
		key:      key,
		val:      val,
		parent:   parent,
		children: make(map[K]*treeNode[K, V]),
	}
}

func (n *treeNode[K, V]) removeFromParent() {
	if n.parent == nil {
		return
	}
	delete(n.parent.children, n.key)
	n.parent = nil
}

func (n *treeNode[K, V]) parentKey() K {
	if n.parent != nil {
		return n.parent.key
	}
	var zeroKey K
	return zeroKey
}

type CacheOption[K comparable, V any] func(*Cache[K, V])

func WithOnEvict[K comparable, V any](onEvict func(node CacheNode[K, V])) CacheOption[K, V] {
	return func(c *Cache[K, V]) {
		c.onEvict = onEvict
	}
}

// WithStatsCollector sets a stats collector for the cache.
func WithStatsCollector[K comparable, V any](stats StatsCollector) CacheOption[K, V] {
	return func(c *Cache[K, V]) {
		c.stats = stats
	}
}

// NewCache creates a new cache with the given maximum number of entries and eviction callback.
func NewCache[K comparable, V any](maxEntries int, options ...CacheOption[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		maxEntries: maxEntries,
		keysMap:    make(map[K]*treeNode[K, V]),
		lruList:    list.New(),
		stats:      nullStats{}, // Use null object by default
	}
	for _, opt := range options {
		opt(c)
	}
	return c
}

// Peek returns the value of the node with the given key without updating the LRU order.
//
// This is useful for checking if a value exists without affecting its position in the eviction order.
// Unlike Get(), this method doesn't mark the node as recently used.
func (c *Cache[K, V]) Peek(key K) (CacheNode[K, V], bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return CacheNode[K, V]{}, false
	}

	c.stats.IncHits()
	return CacheNode[K, V]{Key: key, Value: node.val, ParentKey: node.parentKey()}, true
}

// Get retrieves a value from the cache and updates LRU order.
//
// This method has a side effect of marking the node and all its ancestors as recently used,
// moving them to the front of the LRU list and protecting them from immediate eviction.
func (c *Cache[K, V]) Get(key K) (CacheNode[K, V], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return CacheNode[K, V]{}, false
	}

	// Update LRU order for the node and all its ancestors.
	for n := node; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}

	c.stats.IncHits()
	return CacheNode[K, V]{Key: key, Value: node.val, ParentKey: node.parentKey()}, true
}

// Len returns the number of items currently stored in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.keysMap)
}

// AddRoot initializes the cache with a root node.
//
// The root node serves as the ancestor for all other nodes in the cache.
// Only one root node is allowed per cache instance.
// Attempting to add a second root will result in an error.
func (c *Cache[K, V]) AddRoot(key K, val V) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.root != nil {
		return ErrRootAlreadyExists
	}
	c.root = newTreeNode(key, val, nil)
	c.root.lruElem = c.lruList.PushFront(c.root)
	c.keysMap[key] = c.root

	c.stats.SetAmount(len(c.keysMap))
	return nil
}

// Add inserts a new node into the cache as a child of the specified parent.
//
// This method creates a parent-child relationship between the new node and
// the existing parent node. It also updates the LRU order of the parent chain
// to protect ancestors from eviction. If adding the new node exceeds the cache
// capacity, the least recently used node will be evicted.
//
// If parentKey is not found in the cache, ErrParentNotExist is returned.
// If the node with the given key already exists, ErrAlreadyExists is returned.
func (c *Cache[K, V]) Add(key K, val V, parentKey K) error {
	var evictedNode CacheNode[K, V]
	var evicted bool
	defer func() {
		if evicted && c.onEvict != nil {
			c.onEvict(evictedNode)
		}
	}()

	c.mu.Lock()
	defer c.mu.Unlock()

	parent, parentExists := c.keysMap[parentKey]
	if !parentExists {
		return ErrParentNotExist
	}

	if _, exists := c.keysMap[key]; exists {
		return ErrAlreadyExists
	}

	node := newTreeNode(key, val, parent)
	c.keysMap[key] = node
	node.lruElem = c.lruList.PushFront(node)
	parent.children[key] = node

	for n := node.parent; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}

	if c.maxEntries > 0 && c.lruList.Len() > c.maxEntries {
		evictedNode, evicted = c.evict()
	}

	c.stats.SetAmount(len(c.keysMap))

	return nil
}

// AddOrUpdate adds a new node or updates an existing node in the cache.
//
// This method is more flexible than Add() because it handles both insertion and
// update scenarios. If the node already exists, it can be reparented to a new parent
// and its value can be updated. This method includes cycle detection to prevent
// creating loops in the tree structure (ErrCycleDetected is returned in such cases).
// If parentKey is not found in the cache, ErrParentNotExist is returned.
func (c *Cache[K, V]) AddOrUpdate(key K, val V, parentKey K) error {
	var evictedNode CacheNode[K, V]
	var evicted bool
	defer func() {
		if evicted && c.onEvict != nil {
			c.onEvict(evictedNode)
		}
	}()

	c.mu.Lock()
	defer c.mu.Unlock()

	parent, parentExists := c.keysMap[parentKey]
	if !parentExists {
		return ErrParentNotExist
	}

	node, exists := c.keysMap[key]
	if exists {
		if node.parent != parent {
			// We need to check for cycles before moving the node to the new parent.
			for par := parent; par != nil; par = par.parent {
				if par == node {
					return ErrCycleDetected
				}
			}
			// Before updating the parent, remove the node from the current parent's children.
			node.removeFromParent()
			node.parent = parent
			parent.children[key] = node
		}
		node.val = val
		c.lruList.MoveToFront(node.lruElem)
	} else {
		// Add the new node to the cache.
		node = newTreeNode(key, val, parent)
		c.keysMap[key] = node
		node.lruElem = c.lruList.PushFront(node)
		parent.children[key] = node
	}

	for n := node.parent; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}

	if c.maxEntries > 0 && c.lruList.Len() > c.maxEntries {
		evictedNode, evicted = c.evict()
	}

	c.stats.SetAmount(len(c.keysMap))

	return nil
}

// PeekBranch returns the path from the root to the specified key as a slice of CacheNodes
// without updating the LRU order.
//
// The returned slice is ordered from root (index 0) to the target node (last index).
// If the key does not exist, an empty slice is returned.
// Unlike GetBranch(), this method doesn't mark the nodes as recently used.
func (c *Cache[K, V]) PeekBranch(key K) []CacheNode[K, V] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return nil
	}

	depth := 0
	for n := node; n != nil; n = n.parent {
		depth++
	}
	branch := make([]CacheNode[K, V], depth)
	i := depth
	for n := node; n != nil; n = n.parent {
		i--
		branch[i] = CacheNode[K, V]{Key: n.key, Value: n.val, ParentKey: n.parentKey()}
	}

	c.stats.IncHits()

	return branch
}

// GetBranch returns the path from the root to the specified key as a slice of BranchNodes.
//
// The returned slice is ordered from root (index 0) to the target node (last index).
// If the key does not exist, an empty slice is returned.
// Method updates LRU order for all nodes in the branch.
func (c *Cache[K, V]) GetBranch(key K) []CacheNode[K, V] {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return nil
	}

	depth := 0
	for n := node; n != nil; n = n.parent {
		depth++
	}
	branch := make([]CacheNode[K, V], depth)
	i := depth
	for n := node; n != nil; n = n.parent {
		i--
		branch[i] = CacheNode[K, V]{Key: n.key, Value: n.val, ParentKey: n.parentKey()}
		c.lruList.MoveToFront(n.lruElem)
	}

	c.stats.IncHits()

	return branch
}

// TraverseToRoot walks the path from the specified node up to the root node,
// calling the provided function for each node along the way.
//
// This method traverses the ancestor chain starting from the given node and
// proceeding upward to the root. Each node visited is marked as recently used.
// The provided callback function receives the node's key, value, and its parent's key.
//
// Note: This operation is performed under a lock and will block other cache operations.
// The callback should execute quickly to avoid holding the lock for too long.
func (c *Cache[K, V]) TraverseToRoot(key K, f func(key K, val V, parentKey K)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return
	}

	defer func() {
		// We need to update LRU in defer to ensure that the order is correct even if f panics.
		for n := node; n != nil; n = n.parent {
			c.lruList.MoveToFront(n.lruElem)
		}
	}()

	for n := node; n != nil; n = n.parent {
		var parentKey K
		if n.parent != nil {
			parentKey = n.parent.key
		}
		f(n.key, n.val, parentKey)
	}

	c.stats.IncHits()
}

// TraverseSubtreeOption represents options for the TraverseSubtree method.
type TraverseSubtreeOption func(*traverseOptions)

// WithMaxDepth limits the depth of traversal in TraverseSubtree.
// A depth of -1 means unlimited depth (traverse the entire subtree).
// A depth of 0 means only the specified node (no children).
// A depth of 1 means the node and its immediate children, and so on.
func WithMaxDepth(depth int) TraverseSubtreeOption {
	return func(opts *traverseOptions) {
		opts.maxDepth = depth
	}
}

type traverseOptions struct {
	maxDepth int // -1 means unlimited
}

// TraverseSubtree performs a depth-first traversal of all nodes in the subtree
// rooted at the specified node, with optional depth limitation.
//
// This method visits the specified node and all its descendants in a pre-order depth-first traversal.
// Each node visited is marked as recently used.
// The provided callback function receives the node's key, value, and its parent's key.
//
// Options:
//   - WithMaxDepth(n): Limits traversal to n levels deep.
//
// Note: This operation is performed under a lock and will block other cache operations.
// For large subtrees, this can have performance implications.
func (c *Cache[K, V]) TraverseSubtree(key K, f func(key K, val V, parentKey K), options ...TraverseSubtreeOption) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.keysMap[key]
	if !exists {
		c.stats.IncMisses()
		return
	}

	opts := traverseOptions{
		maxDepth: -1, // Default: unlimited depth
	}
	for _, opt := range options {
		opt(&opts)
	}

	defer func() {
		// We need to update LRU in defer to ensure that the order is correct even if f panics.
		for n := node.parent; n != nil; n = n.parent {
			c.lruList.MoveToFront(n.lruElem)
		}
	}()

	var traverse func(n *treeNode[K, V], currentDepth int)
	traverse = func(n *treeNode[K, V], currentDepth int) {
		defer c.lruList.MoveToFront(n.lruElem)
		var parentKey K
		if n.parent != nil {
			parentKey = n.parent.key
		}
		f(n.key, n.val, parentKey)

		// Check if we need to continue traversing deeper
		if opts.maxDepth >= 0 && currentDepth >= opts.maxDepth {
			return
		}

		for _, child := range n.children {
			traverse(child, currentDepth+1)
		}
	}
	traverse(node, 0) // Start at depth 0 (root of subtree)

	c.stats.IncHits()
}

// Remove deletes a node and all its descendants from the cache.
//
// This method performs a recursive removal of the specified node and its entire subtree.
// It returns the total number of nodes removed from the cache.
func (c *Cache[K, V]) Remove(key K) (removedCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.keysMap[key]
	if !exists {
		return 0
	}

	var removeRecursively func(n *treeNode[K, V])
	removeRecursively = func(n *treeNode[K, V]) {
		delete(c.keysMap, n.key)
		n.parent = nil
		removedCount++
		c.lruList.Remove(n.lruElem)
		for _, child := range n.children {
			removeRecursively(child)
		}
		n.children = nil
	}
	removeRecursively(node)

	node.removeFromParent()

	c.stats.SetAmount(len(c.keysMap))

	return removedCount
}

func (c *Cache[K, V]) evict() (CacheNode[K, V], bool) {
	tailElem := c.lruList.Back()
	if tailElem == nil {
		return CacheNode[K, V]{}, false
	}

	c.lruList.Remove(tailElem)
	node := tailElem.Value.(*treeNode[K, V])
	parentKey := node.parentKey()
	delete(c.keysMap, node.key)
	node.removeFromParent()

	return CacheNode[K, V]{Key: node.key, Value: node.val, ParentKey: parentKey}, true
}

// nullStats is a null object implementation of the StatsCollector interface.
type nullStats struct{}

func (ns nullStats) SetAmount(int)    {}
func (ns nullStats) IncHits()         {}
func (ns nullStats) IncMisses()       {}
func (ns nullStats) AddEvictions(int) {}
