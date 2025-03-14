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
	onEvict    func(key K, value V)
	mu         sync.RWMutex
	m          map[K]*cacheNode[K, V]
	lruList    *list.List
	root       *cacheNode[K, V]
}

type cacheNode[K comparable, V any] struct {
	key      K
	val      V
	parent   *cacheNode[K, V]
	children []*cacheNode[K, V]
	lruElem  *list.Element
}

func (n *cacheNode[K, V]) removeFromParent() {
	if n.parent == nil {
		return
	}
	j := 0
	for i, child := range n.parent.children {
		n.parent.children[i] = n.parent.children[j]
		if child != n {
			j++
		}
	}
	n.parent.children = n.parent.children[:j]
	n.parent = nil
}

type CacheOption[K comparable, V any] func(*Cache[K, V])

func WithOnEvict[K comparable, V any](onEvict func(key K, value V)) CacheOption[K, V] {
	return func(c *Cache[K, V]) {
		c.onEvict = onEvict
	}
}

// NewCache creates a new cache with the given maximum number of entries and eviction callback.
func NewCache[K comparable, V any](maxEntries int, options ...CacheOption[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		maxEntries: maxEntries,
		m:          make(map[K]*cacheNode[K, V]),
		lruList:    list.New(),
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
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.m[key]
	if !exists {
		var zeroVal V
		return zeroVal, false
	}
	return node.val, true
}

// Get retrieves a value from the cache and updates LRU order.
//
// This method has a side effect of marking the node and all its ancestors as recently used,
// moving them to the front of the LRU list and protecting them from immediate eviction.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.m[key]
	if !exists {
		var zeroVal V
		return zeroVal, false
	}

	// Update LRU order for the node and all its ancestors.
	for n := node; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}
	return node.val, true
}

// Len returns the number of items currently stored in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.m)
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
	c.root = &cacheNode[K, V]{key: key, val: val}
	c.root.lruElem = c.lruList.PushFront(c.root)
	c.m[key] = c.root
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
	c.mu.Lock()
	defer c.mu.Unlock()

	parent, parentExists := c.m[parentKey]
	if !parentExists {
		return ErrParentNotExist
	}

	if _, exists := c.m[key]; exists {
		return ErrAlreadyExists
	}

	node := &cacheNode[K, V]{key: key, val: val, parent: parent}
	c.m[key] = node
	node.lruElem = c.lruList.PushFront(node)
	parent.children = append(parent.children, node)

	for n := node.parent; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}

	if c.maxEntries > 0 && c.lruList.Len() > c.maxEntries {
		c.evict()
	}

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
	c.mu.Lock()
	defer c.mu.Unlock()

	parent, parentExists := c.m[parentKey]
	if !parentExists {
		return ErrParentNotExist
	}

	node, exists := c.m[key]
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
			parent.children = append(parent.children, node)
		}
		node.val = val
		c.lruList.MoveToFront(node.lruElem)
	} else {
		// Add the new node to the cache.
		node = &cacheNode[K, V]{key: key, val: val, parent: parent}
		c.m[key] = node
		node.lruElem = c.lruList.PushFront(node)
		parent.children = append(parent.children, node)
	}

	for n := node.parent; n != nil; n = n.parent {
		c.lruList.MoveToFront(n.lruElem)
	}

	if c.maxEntries > 0 && c.lruList.Len() > c.maxEntries {
		c.evict()
	}

	return nil
}

// BranchNode represents a node in a branch path from root to a specific node
type BranchNode[K comparable, V any] struct {
	Key   K
	Value V
}

// GetBranch returns the path from the root to the specified key as a slice of BranchNodes.
//
// The returned slice is ordered from root (index 0) to the target node (last index).
// If the key does not exist, an empty slice is returned.
// Method updates LRU order for all nodes in the branch.
func (c *Cache[K, V]) GetBranch(key K) []BranchNode[K, V] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.m[key]
	if !exists {
		return nil
	}

	depth := 0
	for n := node; n != nil; n = n.parent {
		depth++
	}
	branch := make([]BranchNode[K, V], depth)
	i := depth
	for n := node; n != nil; n = n.parent {
		i--
		branch[i] = BranchNode[K, V]{Key: n.key, Value: n.val}
		c.lruList.MoveToFront(n.lruElem)
	}

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

	node, exists := c.m[key]
	if !exists {
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
}

// TraverseSubtree performs a depth-first traversal of all nodes in the subtree
// rooted at the specified node.
//
// This method visits the specified node and all its descendants in a pre-order depth-first traversal.
// Each node visited is marked as recently used.
// The provided callback function receives the node's key, value, and its parent's key.
//
// Note: This operation is performed under a lock and will block other cache operations.
// For large subtrees, this can have performance implications.
func (c *Cache[K, V]) TraverseSubtree(key K, f func(key K, val V, parentKey K)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.m[key]
	if !exists {
		return
	}

	defer func() {
		// We need to update LRU in defer to ensure that the order is correct even if f panics.
		for n := node.parent; n != nil; n = n.parent {
			c.lruList.MoveToFront(n.lruElem)
		}
	}()

	var iterate func(n *cacheNode[K, V])
	iterate = func(n *cacheNode[K, V]) {
		defer c.lruList.MoveToFront(n.lruElem)
		var parentKey K
		if n.parent != nil {
			parentKey = n.parent.key
		}
		f(n.key, n.val, parentKey)
		for _, child := range n.children {
			iterate(child)
		}
	}
	iterate(node)
}

// Remove deletes a node and all its descendants from the cache.
//
// This method performs a recursive removal of the specified node and its entire subtree.
// It returns the total number of nodes removed from the cache.
func (c *Cache[K, V]) Remove(key K) (removedCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.m[key]
	if !exists {
		return 0
	}

	var removeRecursively func(n *cacheNode[K, V])
	removeRecursively = func(n *cacheNode[K, V]) {
		delete(c.m, n.key)
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

	return removedCount
}

func (c *Cache[K, V]) evict() {
	tailElem := c.lruList.Back()
	if tailElem == nil {
		return
	}

	c.lruList.Remove(tailElem)
	node := tailElem.Value.(*cacheNode[K, V])
	delete(c.m, node.key)
	node.removeFromParent()

	if c.onEvict != nil {
		c.onEvict(node.key, node.val)
	}
}
