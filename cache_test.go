package lrutree

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCache_AddRoot(t *testing.T) {
	cache := NewCache[string, int](10)
	err := cache.AddRoot("root", 42)
	assertNoError(t, err)
	assertEqual(t, 1, cache.Len())
	assertEqual(t, []string{"root"}, getLRUOrder(cache))

	// Verify the root node is added correctly.
	cacheNode, ok := cache.Peek("root")
	assertTrue(t, ok)
	assertEqual(t, CacheNode[string, int]{Key: "root", Value: 42}, cacheNode)

	// Verify that adding a root node again returns an error.
	err = cache.AddRoot("root", 2)
	assertErrorIs(t, err, ErrRootAlreadyExists)
}

func TestCache_Add(t *testing.T) {
	var lastEvicted *CacheNode[string, int]
	onEvict := func(node CacheNode[string, int]) {
		lastEvicted = &node
	}
	cache := NewCache[string, int](12, WithOnEvict(onEvict))

	assertNoError(t, cache.AddRoot("root", 1))

	// Add a child node to the root.
	lastEvicted = nil
	err := cache.Add("sub-root-1", 2, "root")
	assertNoError(t, err)
	assertNil(t, lastEvicted)
	assertEqual(t, []string{"root", "sub-root-1"}, getLRUOrder(cache))

	// Verify the child node is added correctly.
	cacheNode, ok := cache.Peek("sub-root-1")
	assertTrue(t, ok)
	assertEqual(t, CacheNode[string, int]{Key: "sub-root-1", Value: 2, ParentKey: "root"}, cacheNode)
	assertEqual(t, 2, cache.Len())

	// Verify that adding a node with an existing key returns an error.
	err = cache.Add("sub-root-1", 3, "root")
	assertErrorIs(t, err, ErrAlreadyExists)

	// Verify that adding a node with a non-existent parent returns an error.
	err = cache.Add("sub-root-2", 3, "nonexistent")
	assertErrorIs(t, err, ErrParentNotExist)

	// Verify eviction on adding a new node.
	lastEvicted = nil
	for i := 1; i <= 5; i++ {
		partnerKey := "partner-" + strconv.Itoa(i)
		assertNoError(t, cache.Add(partnerKey, 10+i, "sub-root-1"))
		assertNoError(t, cache.Add("customer-"+strconv.Itoa(i), 100+i, partnerKey))
	}
	assertNil(t, lastEvicted)
	assertEqual(t, []string{
		"root", "sub-root-1", "partner-5", "customer-5", "partner-4", "customer-4", "partner-3", "customer-3",
		"partner-2", "customer-2", "partner-1", "customer-1",
	}, getLRUOrder(cache))
	// Tree:
	// root
	// 	sub-root-1
	// 		partner-1
	// 			customer-1
	// 		partner-2
	// 			customer-2
	// 		partner-3
	// 			customer-3
	// 		partner-4
	// 			customer-4
	// 		partner-5
	// 			customer-5

	assertNoError(t, cache.Add("partner-6", 16, "sub-root-1"))
	assertEqual(t, &CacheNode[string, int]{Key: "customer-1", Value: 101, ParentKey: "partner-1"}, lastEvicted)
	assertEqual(t, []string{
		"root", "sub-root-1", "partner-6", "partner-5", "customer-5", "partner-4", "customer-4",
		"partner-3", "customer-3", "partner-2", "customer-2", "partner-1",
	}, getLRUOrder(cache))
	// Tree:
	// root
	// 	sub-root-1
	// 		partner-1
	// 		partner-2
	// 			customer-2
	// 		partner-3
	// 			customer-3
	// 		partner-4
	// 			customer-4
	// 		partner-5
	// 			customer-5
	// 		partner-6

	assertNoError(t, cache.Add("customer-6", 106, "partner-6"))
	assertEqual(t, &CacheNode[string, int]{Key: "partner-1", Value: 11, ParentKey: "sub-root-1"}, lastEvicted)
	assertEqual(t, []string{
		"root", "sub-root-1", "partner-6", "customer-6", "partner-5", "customer-5", "partner-4", "customer-4",
		"partner-3", "customer-3", "partner-2", "customer-2",
	}, getLRUOrder(cache))
	// Tree:
	// root
	// 	sub-root-1
	// 		partner-2
	// 			customer-2
	// 		partner-3
	// 			customer-3
	// 		partner-4
	// 			customer-4
	// 		partner-5
	// 			customer-5
	// 		partner-6
	// 			customer-6

	// Touch the customer-2 node to move it to the front of the LRU list.
	cacheNode, ok = cache.Get("customer-2")
	assertTrue(t, ok)
	assertEqual(t, CacheNode[string, int]{Key: "customer-2", Value: 102, ParentKey: "partner-2"}, cacheNode)
	assertEqual(t, []string{
		"root", "sub-root-1", "partner-2", "customer-2", "partner-6", "customer-6", "partner-5", "customer-5",
		"partner-4", "customer-4", "partner-3", "customer-3",
	}, getLRUOrder(cache))

	// Verify eviction on adding a new node after touching an existing node.
	assertNoError(t, cache.Add("partner-7", 17, "sub-root-1"))
	assertEqual(t, &CacheNode[string, int]{Key: "customer-3", Value: 103, ParentKey: "partner-3"}, lastEvicted)
	assertEqual(t, []string{
		"root", "sub-root-1", "partner-7", "partner-2", "customer-2", "partner-6", "customer-6", "partner-5",
		"customer-5", "partner-4", "customer-4", "partner-3",
	}, getLRUOrder(cache))
	// Tree:
	// root
	// 	sub-root-1
	// 		partner-2
	// 			customer-2
	// 		partner-3
	// 		partner-4
	// 			customer-4
	// 		partner-5
	// 			customer-5
	// 		partner-6
	// 			customer-6
	// 		partner-7
}

func TestCache_Remove(t *testing.T) {
	t.Run("removing leaf node", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("sub-root", 2, "root"))
		assertEqual(t, []string{"root", "sub-root"}, getLRUOrder(cache))

		removedCount := cache.Remove("sub-root")
		assertEqual(t, 1, removedCount)
		assertEqual(t, 1, cache.Len())
		_, ok := cache.Peek("sub-root")
		assertFalse(t, ok)
		assertEqual(t, []string{"root"}, getLRUOrder(cache))
	})

	t.Run("removing non-leaf node", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("sub-root", 2, "root"))
		assertNoError(t, cache.Add("partner-1", 3, "sub-root"))
		assertNoError(t, cache.Add("customer-1", 4, "partner-1"))
		assertNoError(t, cache.Add("partner-2", 5, "sub-root"))
		assertNoError(t, cache.Add("customer-2", 6, "partner-2"))
		assertEqual(t, []string{
			"root", "sub-root", "partner-2", "customer-2", "partner-1", "customer-1",
		}, getLRUOrder(cache))

		removedCount := cache.Remove("sub-root")
		assertEqual(t, 5, removedCount)
		assertEqual(t, 1, cache.Len())
		for _, key := range []string{"sub-root", "partner-1", "customer-1", "partner-2", "customer-2"} {
			_, ok := cache.Peek(key)
			assertFalse(t, ok)
		}
		assertEqual(t, []string{"root"}, getLRUOrder(cache))
	})

	t.Run("removing root node", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("sub-root", 2, "root"))
		assertNoError(t, cache.Add("partner-1", 3, "sub-root"))
		assertNoError(t, cache.Add("customer-1", 4, "partner-1"))
		assertNoError(t, cache.Add("partner-2", 5, "sub-root"))
		assertNoError(t, cache.Add("customer2", 6, "partner-2"))

		removedCount := cache.Remove("root")
		assertEqual(t, 6, removedCount)
		assertEqual(t, 0, cache.Len())
		for _, key := range []string{"root", "sub-root", "partner-1", "customer-1", "partner-2", "customer-2"} {
			_, ok := cache.Peek(key)
			assertFalse(t, ok)
		}
		assertEqual(t, 0, len(getLRUOrder(cache)))
	})

	t.Run("removing non-existent node", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("sub-root", 2, "root"))

		removedCount := cache.Remove("nonexistent")
		assertEqual(t, 0, removedCount)
		assertEqual(t, 2, cache.Len())
	})
}

func TestCache_AddOrUpdate(t *testing.T) {
	t.Run("new node", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		// Add a new node using AddOrUpdate.
		assertNoError(t, cache.AddOrUpdate("child", 2, "root"))
		assertEqual(t, []string{"root", "child"}, getLRUOrder(cache))
		cacheNode, ok := cache.Peek("child")
		assertTrue(t, ok)
		assertEqual(t, CacheNode[string, int]{Key: "child", Value: 2, ParentKey: "root"}, cacheNode)
	})

	t.Run("update value with same parent", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child", 2, "root"))
		// Update the value while keeping the same parent.
		assertNoError(t, cache.AddOrUpdate("child", 20, "root"))
		assertEqual(t, []string{"root", "child"}, getLRUOrder(cache))
		cacheNode, ok := cache.Peek("child")
		assertTrue(t, ok)
		assertEqual(t, CacheNode[string, int]{Key: "child", Value: 20, ParentKey: "root"}, cacheNode)
	})

	t.Run("update parent", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		// Reparent child1 from "root" to "child2" using AddOrUpdate.
		assertNoError(t, cache.AddOrUpdate("child1", 22, "child2"))
		assertEqual(t, []string{"root", "child2", "child1"}, getLRUOrder(cache))
		var traversed []CacheNode[string, int]
		cache.TraverseToRoot("child1", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})
		expected := []CacheNode[string, int]{
			{Key: "child1", Value: 22, ParentKey: "child2"},
			{Key: "child2", Value: 3, ParentKey: "root"},
			{Key: "root", Value: 1, ParentKey: ""},
		}
		assertEqual(t, expected, traversed)
	})

	t.Run("cycle detection", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child", 2, "root"))
		assertNoError(t, cache.Add("grandchild", 3, "child"))
		// Attempt to update "root" to be a child of "grandchild", which should create a cycle.
		err := cache.AddOrUpdate("root", 10, "grandchild")
		assertErrorIs(t, err, ErrCycleDetected)
	})

	t.Run("invalid parent", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		// Trying to add/update a node with a non-existent parent should return an error.
		err := cache.AddOrUpdate("child", 2, "nonexistent")
		assertErrorIs(t, err, ErrParentNotExist)
	})
}

func TestCache_GetBranch(t *testing.T) {
	t.Run("key exists", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 10))
		assertNoError(t, cache.Add("child1", 20, "root"))
		assertNoError(t, cache.Add("grandchild1", 30, "child1"))
		assertNoError(t, cache.Add("child2", 40, "root"))
		assertNoError(t, cache.Add("grandchild2", 50, "child2"))
		assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))

		assertEqual(t, []CacheNode[string, int]{
			{Key: "root", Value: 10},
			{Key: "child1", Value: 20, ParentKey: "root"},
			{Key: "grandchild1", Value: 30, ParentKey: "child1"},
		}, cache.GetBranch("grandchild1"))
		assertEqual(t, []string{"root", "child1", "grandchild1", "child2", "grandchild2"}, getLRUOrder(cache))
	})

	t.Run("key is root", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 10))
		assertNoError(t, cache.Add("level1", 20, "root"))
		assertEqual(t, []CacheNode[string, int]{{Key: "root", Value: 10}}, cache.GetBranch("root"))
	})

	t.Run("key doesn't exist", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertEqual(t, 0, len(cache.GetBranch("nonexistent")))
	})

	t.Run("empty cache", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertEqual(t, 0, len(cache.GetBranch("anything")))
	})
}

func TestCache_TraverseToRoot(t *testing.T) {
	t.Run("key not found", func(t *testing.T) {
		cache := NewCache[string, int](10)
		_, ok := cache.Get("nonexistent")
		assertFalse(t, ok)

		var traversed []CacheNode[string, int]
		cache.TraverseToRoot("nonexistent", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})
		assertEqual(t, 0, len(traversed))
	})

	t.Run("key found, LRU list updated", func(t *testing.T) {
		var lastEvicted *CacheNode[string, int]
		onEvict := func(node CacheNode[string, int]) {
			lastEvicted = &node
		}
		cache := NewCache[string, int](3, WithOnEvict(onEvict))
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("sub-root", 2, "root"))
		assertNoError(t, cache.Add("partner-1", 3, "sub-root"))
		assertNil(t, lastEvicted)

		// Get the "partner" node.
		cacheNode, ok := cache.Get("partner-1")
		assertTrue(t, ok)
		assertEqual(t, CacheNode[string, int]{Key: "partner-1", Value: 3, ParentKey: "sub-root"}, cacheNode)

		// Traverse to the root node.
		var traversed []CacheNode[string, int]
		cache.TraverseToRoot("partner-1", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})
		expected := []CacheNode[string, int]{
			{Key: "partner-1", Value: 3, ParentKey: "sub-root"},
			{Key: "sub-root", Value: 2, ParentKey: "root"},
			{Key: "root", Value: 1, ParentKey: ""},
		}
		assertEqual(t, expected, traversed)

		// Verify the LRU list is updated properly. The customer node will be evicted immediately.
		assertNoError(t, cache.Add("customer-1", 4, "partner-1"))
		_, ok = cache.Get("customer-1")
		assertFalse(t, ok)
		for _, key := range []string{"root", "sub-root", "partner-1"} {
			_, ok = cache.Get(key)
			assertTrue(t, ok)
		}
		assertEqual(t, &CacheNode[string, int]{Key: "customer-1", Value: 4, ParentKey: "partner-1"}, lastEvicted)
		assertEqual(t, 3, cache.Len())

		// Add a new node to the cache under the "sub-root" node. Partner-1 node will be evicted.
		assertNoError(t, cache.Add("customer-2", 5, "sub-root"))
		_, ok = cache.Get("partner-1")
		assertFalse(t, ok)
		for _, key := range []string{"root", "sub-root", "customer-2"} {
			_, ok = cache.Get(key)
			assertTrue(t, ok)
		}
		assertEqual(t, CacheNode[string, int]{Key: "partner-1", Value: 3, ParentKey: "sub-root"}, cacheNode)
		assertEqual(t, 3, cache.Len())
	})

	t.Run("panicking callback", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("grandchild1", 3, "child1"))
		assertNoError(t, cache.Add("child2", 4, "root"))
		assertNoError(t, cache.Add("grandchild2", 5, "child2"))
		assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))

		panicFunc := func(key string, val int, parentKey string) {
			panic("intentional panic in callback")
		}

		defer func() {
			r := recover()
			assertEqual(t, "intentional panic in callback", r.(string))

			// Verify that LRU order was updated in case of panic
			assertEqual(t, []string{"root", "child1", "grandchild1", "child2", "grandchild2"}, getLRUOrder(cache))

			// Verify that the cache is still usable after panic
			cacheNode, ok := cache.Get("grandchild2")
			assertTrue(t, ok)
			assertEqual(t, CacheNode[string, int]{Key: "grandchild2", Value: 5, ParentKey: "child2"}, cacheNode)
			assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))
		}()

		cache.TraverseToRoot("grandchild1", panicFunc)
	})
}

func TestCache_TraverseSubtree(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		assertNoError(t, cache.Add("grandchild1", 4, "child1"))

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("child1", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})
		// Expected pre-order: starting at "child1" then "grandchild1"
		assertEqual(t, []CacheNode[string, int]{
			{Key: "child1", Value: 2, ParentKey: "root"},
			{Key: "grandchild1", Value: 4, ParentKey: "child1"},
		}, traversed)
		assertEqual(t, []string{"root", "child1", "grandchild1", "child2"}, getLRUOrder(cache))
	})

	t.Run("multiple children", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		assertNoError(t, cache.Add("grandchild1", 4, "child1"))
		assertNoError(t, cache.Add("grandchild2", 5, "child2"))

		var traversed []CacheNode[string, int]
		// Iterating from the root should traverse the whole tree in depth-first order.
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})

		assertEqual(t, 5, len(traversed))
		if traversed[len(traversed)-1].Key == "grandchild2" {
			assertEqual(t, []CacheNode[string, int]{
				{Key: "root", Value: 1, ParentKey: ""},
				{Key: "child1", Value: 2, ParentKey: "root"},
				{Key: "grandchild1", Value: 4, ParentKey: "child1"},
				{Key: "child2", Value: 3, ParentKey: "root"},
				{Key: "grandchild2", Value: 5, ParentKey: "child2"},
			}, traversed)
			assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))
		} else {
			assertEqual(t, []CacheNode[string, int]{
				{Key: "root", Value: 1, ParentKey: ""},
				{Key: "child2", Value: 3, ParentKey: "root"},
				{Key: "grandchild2", Value: 5, ParentKey: "child2"},
				{Key: "child1", Value: 2, ParentKey: "root"},
				{Key: "grandchild1", Value: 4, ParentKey: "child1"},
			}, traversed)
			assertEqual(t, []string{"root", "child1", "grandchild1", "child2", "grandchild2"}, getLRUOrder(cache))
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		cache := NewCache[string, int](10)
		var iterated []string
		// Calling TraverseSubtree on a non-existent key should not invoke the callback.
		cache.TraverseSubtree("nonexistent", func(key string, val int, parentKey string) {
			iterated = append(iterated, key)
		})
		// Calling TraverseSubtree on a non-existent key should not invoke the callback.
		assertEqual(t, 0, len(iterated))
	})

	t.Run("panicking callback", func(t *testing.T) {
		cache := NewCache[string, int](10)
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("grandchild1", 3, "child1"))
		assertNoError(t, cache.Add("child2", 4, "root"))
		assertNoError(t, cache.Add("grandchild2", 5, "child2"))
		assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))

		// Create a callback function that will panic
		panicFunc := func(key string, val int, parentKey string) {
			panic("intentional panic in subtree traversal")
		}

		// The panic should be recovered by the test
		defer func() {
			r := recover()
			assertEqual(t, "intentional panic in subtree traversal", r.(string))

			assertEqual(t, []string{"root", "child1", "child2", "grandchild2", "grandchild1"}, getLRUOrder(cache))

			// Verify that the cache is still usable after panic
			cacheNode, ok := cache.Get("grandchild2")
			assertTrue(t, ok)
			assertEqual(t, CacheNode[string, int]{Key: "grandchild2", Value: 5, ParentKey: "child2"}, cacheNode)
			assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1"}, getLRUOrder(cache))
		}()

		cache.TraverseSubtree("child1", panicFunc)
	})
}

func TestCache_TraverseSubtree_WithMaxDepth(t *testing.T) {
	nodes := map[string]CacheNode[string, int]{
		"root":             {Key: "root", Value: 11, ParentKey: ""},
		"child1":           {Key: "child1", Value: 12, ParentKey: "root"},
		"child2":           {Key: "child2", Value: 13, ParentKey: "root"},
		"grandchild1":      {Key: "grandchild1", Value: 14, ParentKey: "child1"},
		"grandchild2":      {Key: "grandchild2", Value: 15, ParentKey: "child2"},
		"greatgrandchild1": {Key: "greatgrandchild1", Value: 16, ParentKey: "grandchild1"},
		"greatgrandchild2": {Key: "greatgrandchild2", Value: 17, ParentKey: "grandchild2"},
	}

	setupCache := func() *Cache[string, int] {
		cache := NewCache[string, int](10)
		for _, key := range []string{"root", "child1", "child2", "grandchild1", "grandchild2", "greatgrandchild1", "greatgrandchild2"} {
			node := nodes[key]
			if node.ParentKey == "" {
				assertNoError(t, cache.AddRoot(node.Key, node.Value))
			} else {
				assertNoError(t, cache.Add(node.Key, node.Value, node.ParentKey))
			}
		}
		return cache
	}

	makeNodes := func(keys ...string) []CacheNode[string, int] {
		result := make([]CacheNode[string, int], 0, len(keys))
		for _, key := range keys {
			result = append(result, nodes[key])
		}
		return result
	}

	t.Run("with unlimited depth", func(t *testing.T) {
		cache := setupCache()

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		})

		assertEqual(t, 7, len(traversed))
		if traversed[1].Key == "child1" {
			assertEqual(t, makeNodes("root", "child1", "grandchild1", "greatgrandchild1", "child2", "grandchild2", "greatgrandchild2"), traversed)
			assertEqual(t, []string{"root", "child2", "grandchild2", "greatgrandchild2", "child1", "grandchild1", "greatgrandchild1"}, getLRUOrder(cache))
		} else {
			assertEqual(t, makeNodes("root", "child2", "grandchild2", "greatgrandchild2", "child1", "grandchild1", "greatgrandchild1"), traversed)
			assertEqual(t, []string{"root", "child1", "grandchild1", "greatgrandchild1", "child2", "grandchild2", "greatgrandchild2"}, getLRUOrder(cache))
		}
	})

	t.Run("with depth 0 - node only", func(t *testing.T) {
		cache := setupCache()

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		}, WithMaxDepth(0))

		assertEqual(t, 1, len(traversed))
		assertEqual(t, makeNodes("root"), traversed)
		assertEqual(t, []string{"root", "child2", "grandchild2", "greatgrandchild2", "child1", "grandchild1", "greatgrandchild1"}, getLRUOrder(cache))
	})

	t.Run("with depth 1 - node and immediate children", func(t *testing.T) {
		cache := setupCache()

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		}, WithMaxDepth(1))

		assertEqual(t, 3, len(traversed))
		if traversed[1].Key == "child1" {
			assertEqual(t, makeNodes("root", "child1", "child2"), traversed)
			assertEqual(t, []string{"root", "child2", "child1", "grandchild2", "greatgrandchild2", "grandchild1", "greatgrandchild1"}, getLRUOrder(cache))
		} else {
			assertEqual(t, makeNodes("root", "child2", "child1"), traversed)
			assertEqual(t, []string{"root", "child1", "child2", "grandchild2", "greatgrandchild2", "grandchild1", "greatgrandchild1"}, getLRUOrder(cache))
		}
	})

	t.Run("depth 2 - node, children, and grandchildren", func(t *testing.T) {
		cache := setupCache()

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		}, WithMaxDepth(2))

		assertEqual(t, 5, len(traversed))
		if traversed[1].Key == "child1" {
			assertEqual(t, makeNodes("root", "child1", "grandchild1", "child2", "grandchild2"), traversed)
			assertEqual(t, []string{"root", "child2", "grandchild2", "child1", "grandchild1", "greatgrandchild2", "greatgrandchild1"}, getLRUOrder(cache))
		} else {
			assertEqual(t, makeNodes("root", "child2", "grandchild2", "child1", "grandchild1"), traversed)
			assertEqual(t, []string{"root", "child1", "grandchild1", "child2", "grandchild2", "greatgrandchild2", "greatgrandchild1"}, getLRUOrder(cache))
		}
	})

	t.Run("traverse from middle node", func(t *testing.T) {
		cache := setupCache()

		var traversed []CacheNode[string, int]
		cache.TraverseSubtree("child1", func(key string, val int, parentKey string) {
			traversed = append(traversed, CacheNode[string, int]{Key: key, Value: val, ParentKey: parentKey})
		}, WithMaxDepth(1))

		assertEqual(t, makeNodes("child1", "grandchild1"), traversed)
		assertEqual(t, []string{"root", "child1", "grandchild1", "child2", "grandchild2", "greatgrandchild2", "greatgrandchild1"}, getLRUOrder(cache))
	})
}

func TestConcurrency(t *testing.T) {
	cache := NewCache[string, int](100_000)
	assertNoError(t, cache.AddRoot("root", 1))

	// Create a deep tree
	for i := 1; i <= 100; i++ {
		for j := 1; j <= 100; j++ {
			parent := "root"
			if j > 1 {
				parent = fmt.Sprintf("node-%d-%d", i, j-1)
			}
			assertNoError(t, cache.Add(fmt.Sprintf("node-%d-%d", i, j), i*1000+j, parent))
		}
	}

	errs := make(chan error, 10_000)

	// Run concurrent operations including some that will panic
	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		for j := 1; j <= 100; j++ {
			wg.Add(1)
			go func(i, j int) {
				defer wg.Done()
				defer func() {
					_ = recover()
				}()

				key := fmt.Sprintf("node-%d-%d", i, j)

				// Mix of operations, some will panic
				switch j % 5 {
				case 0:
					// Normal get
					cacheNode, ok := cache.Get(key)
					if !ok {
						errs <- fmt.Errorf("%s not found in cache", key)
					}
					if cacheNode.Value != i*1000+j {
						errs <- fmt.Errorf("unexpected value for %s, want %d, got %d", key, i*1000+j, cacheNode.Value)
					}
				case 1:
					// Traverse with panic possibility
					cache.TraverseToRoot(key, func(key string, val int, parentKey string) {
						if j%7 == 0 {
							panic("random panic in TraverseToRoot callback")
						}
					})
				case 2:
					// Traverse subtree with panic possibility
					cache.TraverseSubtree("root", func(key string, val int, parentKey string) {
						if j%8 == 0 {
							panic("random panic in TraverseSubtree callback")
						}
					})
				case 3:
					_ = cache.Add(fmt.Sprintf("temp-node-%d-%d", i, j), i*1000+j, key)
				case 4:
					cache.Remove(fmt.Sprintf("temp-node-%d-%d", i, j))
				}
			}(i, j)
		}
	}
	wg.Wait()

	close(errs)
	for err := range errs {
		assertNoError(t, err)
	}

	// Verify the cache is still in a usable state
	cacheNode, ok := cache.Get("root")
	assertTrue(t, ok)
	assertEqual(t, CacheNode[string, int]{Key: "root", Value: 1}, cacheNode)

	// Verify we can still perform operations
	err := cache.Add("final-test", 999, "root")
	assertNoError(t, err)
	cacheNode, ok = cache.Get("final-test")
	assertTrue(t, ok)
	assertEqual(t, CacheNode[string, int]{Key: "final-test", Value: 999, ParentKey: "root"}, cacheNode)
}

func getLRUOrder[K comparable, V any](c *Cache[K, V]) []K {
	keys := make([]K, 0, c.Len())
	for e := c.lruList.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(*treeNode[K, V]).key)
	}
	return keys
}

// mockStats implements the StatsCollector interface for testing.
type mockStats struct {
	amount    atomic.Int32
	hits      atomic.Int32
	misses    atomic.Int32
	evictions atomic.Int32
}

func (m *mockStats) SetAmount(val int) {
	m.amount.Store(int32(val))
}

func (m *mockStats) IncHits() {
	m.hits.Add(1)
}

func (m *mockStats) IncMisses() {
	m.misses.Add(1)
}

func (m *mockStats) AddEvictions(val int) {
	m.evictions.Add(int32(val))
}

// panicingStats implements StatsCollector but panics on every 2nd call
type panicingStats struct {
	calls atomic.Int32
}

func (p *panicingStats) SetAmount(val int) {
	if p.calls.Add(1)%2 == 0 {
		panic("SetAmount panic")
	}
}

func (p *panicingStats) IncHits() {
	if p.calls.Add(1)%2 == 0 {
		panic("IncHits panic")
	}
}

func (p *panicingStats) IncMisses() {
	if p.calls.Add(1)%2 == 0 {
		panic("IncMisses panic")
	}
}

func (p *panicingStats) AddEvictions(val int) {
	if p.calls.Add(1)%2 == 0 {
		panic("AddEvictions panic")
	}
}

func TestCache_Stats(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		stats := &mockStats{}
		cache := NewCache[string, int](5, WithStatsCollector[string, int](stats))

		// Test adding root and child nodes
		assertNoError(t, cache.AddRoot("root", 1))
		assertEqual(t, int32(1), stats.amount.Load())

		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		assertEqual(t, int32(3), stats.amount.Load())

		// Test hits and misses
		_, ok := cache.Get("child1")
		assertTrue(t, ok)
		assertEqual(t, int32(1), stats.hits.Load())

		_, ok = cache.Get("nonexistent")
		assertFalse(t, ok)
		assertEqual(t, int32(1), stats.misses.Load())

		// Test Peek
		_, ok = cache.Peek("child2")
		assertTrue(t, ok)
		assertEqual(t, int32(2), stats.hits.Load())

		_, ok = cache.Peek("nonexistent2")
		assertFalse(t, ok)
		assertEqual(t, int32(2), stats.misses.Load())
	})

	t.Run("eviction", func(t *testing.T) {
		var lastEvicted *CacheNode[string, int]
		onEvict := func(node CacheNode[string, int]) {
			lastEvicted = &node
		}

		stats := &mockStats{}
		cache := NewCache[string, int](3,
			WithOnEvict(onEvict),
			WithStatsCollector[string, int](stats),
		)

		// Setup the cache
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		assertEqual(t, int32(3), stats.amount.Load())
		assertEqual(t, int32(0), stats.evictions.Load())

		// This should cause eviction
		assertNoError(t, cache.Add("child3", 4, "root"))
		assertEqual(t, int32(3), stats.amount.Load()) // Still 3 items
		assertEqual(t, "child1", lastEvicted.Key)     // child1 was evicted

		// Update LRU order and add another node to cause another eviction
		_, ok := cache.Get("child2")
		assertTrue(t, ok)
		assertNoError(t, cache.Add("child4", 5, "root"))
		assertEqual(t, "child3", lastEvicted.Key) // child3 should be evicted now
	})

	t.Run("subtree operations", func(t *testing.T) {
		stats := &mockStats{}
		cache := NewCache[string, int](10, WithStatsCollector[string, int](stats))

		// Create a tree structure
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		assertNoError(t, cache.Add("child2", 3, "root"))
		assertNoError(t, cache.Add("grandchild1", 4, "child1"))
		assertNoError(t, cache.Add("grandchild2", 5, "child1"))

		// Test branch traversal
		branch := cache.GetBranch("grandchild1")
		assertEqual(t, 3, len(branch)) // root -> child1 -> grandchild1
		assertEqual(t, int32(1), stats.hits.Load())

		// Test TraverseToRoot
		cache.TraverseToRoot("grandchild2", func(key string, val int, parentKey string) {})
		assertEqual(t, int32(2), stats.hits.Load())

		// Test TraverseSubtree
		cache.TraverseSubtree("child1", func(key string, val int, parentKey string) {})
		assertEqual(t, int32(3), stats.hits.Load())

		// Remove a subtree
		removedCount := cache.Remove("child1")
		assertEqual(t, 3, removedCount)               // child1, grandchild1, grandchild2
		assertEqual(t, int32(2), stats.amount.Load()) // root and child2 left
	})

	t.Run("AddOrUpdate", func(t *testing.T) {
		stats := &mockStats{}
		cache := NewCache[string, int](5, WithStatsCollector[string, int](stats))

		// Add root and child
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.AddOrUpdate("child1", 2, "root"))
		assertEqual(t, int32(2), stats.amount.Load())

		// Update existing node
		assertNoError(t, cache.AddOrUpdate("child1", 3, "root"))
		assertEqual(t, int32(2), stats.amount.Load()) // Count should stay the same

		// Add more nodes to test eviction
		for i := 2; i <= 5; i++ {
			assertNoError(t, cache.AddOrUpdate("child"+string(rune('0'+i)), i+1, "root"))
		}
		assertEqual(t, int32(5), stats.amount.Load()) // At capacity
	})

	t.Run("null stats", func(t *testing.T) {
		// Create cache without explicit stats
		cache := NewCache[string, int](5)

		// These operations should not panic
		assertNoError(t, cache.AddRoot("root", 1))
		assertNoError(t, cache.Add("child1", 2, "root"))
		_, _ = cache.Get("child1")
		_, _ = cache.Get("nonexistent")
		_, _ = cache.Peek("child1")
		_ = cache.GetBranch("child1")
		cache.TraverseToRoot("child1", func(key string, val int, parentKey string) {})
		cache.TraverseSubtree("root", func(key string, val int, parentKey string) {})
		_ = cache.Remove("child1")

		// Add nodes until eviction occurs
		for i := 0; i < 10; i++ {
			key := "node" + string(rune('0'+i))
			_ = cache.Add(key, i, "root")
		}

		assertEqual(t, 5, cache.Len()) // Capacity is 5
	})

	t.Run("recovery from stats panic", func(t *testing.T) {
		// Create a stats implementation that panics
		panicStats := &panicingStats{}
		cache := NewCache[string, int](10, WithStatsCollector[string, int](panicStats))

		// Setup cache with some initial data
		assertNoError(t, cache.AddRoot("root", 1))

		// Test that we recover from panic in Add
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "SetAmount panic", r.(string))
			}()
			_ = cache.Add("child1", 2, "root")
		}()
		// Cache should still be usable
		assertNoError(t, cache.Add("child2", 3, "root"))

		// Test we recover from panic in Get
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "IncHits panic", r.(string))
			}()
			_, _ = cache.Get("child1")
		}()
		// Cache should still be usable
		node, exists := cache.Get("child1")
		assertTrue(t, exists)
		assertEqual(t, 2, node.Value)

		// Test we recover from panic in Get for non-existent key
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "IncMisses panic", r.(string))
			}()
			_, _ = cache.Get("non-existent")
		}()
		// Cache should still be usable
		_, exists = cache.Get("non-existent")
		assertFalse(t, exists)

		// Test that we recover from panic in GetBranch
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "IncHits panic", r.(string))
			}()
			_ = cache.GetBranch("child2")
		}()
		// Cache should still be usable
		branch := cache.GetBranch("child2")
		assertEqual(t, 2, len(branch))

		// Test that we recover from panic in AddOrUpdate
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "SetAmount panic", r.(string))
			}()
			_ = cache.AddOrUpdate("child3", 4, "root")
		}()
		// Cache should still be usable
		assertNoError(t, cache.AddOrUpdate("child3", 5, "root"))

		// Test we recover from panic in Remove
		func() {
			defer func() {
				r := recover()
				assertEqual(t, "SetAmount panic", r.(string))
			}()
			_ = cache.Remove("child3")
		}()
		// Cache should still be usable
		count := cache.Remove("child3")
		assertEqual(t, 0, count)
	})
}
