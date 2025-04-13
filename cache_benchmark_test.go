package lrutree

import (
	"fmt"
	"sync"
	"testing"
)

func BenchmarkCache_Get(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		b.Run(fmt.Sprintf("depth=%d/elements=%d", depth, depth*chainsNum+1), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nodeIdx := i % len(leaves)
				key := leaves[nodeIdx]
				if _, found := cache.Get(key); !found {
					b.Fatalf("key %s not found in cache", key)
				}
			}
		})
	}
}

func BenchmarkCache_Peek(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		b.Run(fmt.Sprintf("depth=%d/elements=%d", depth, depth*chainsNum+1), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nodeIdx := i % len(leaves)
				key := leaves[nodeIdx]
				if _, found := cache.Peek(key); !found {
					b.Fatalf("key %s not found in cache", key)
				}
			}
		})
	}
}

func BenchmarkCache_GetBranch(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		b.Run(fmt.Sprintf("depth=%d/elements=%d", depth, depth*chainsNum+1), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nodeIdx := i % len(leaves)
				key := leaves[nodeIdx]
				if branch := cache.GetBranch(key); len(branch) != depth {
					b.Fatalf("branch length %d, expected %d", len(branch), depth)
				}
			}
		})
	}
}

func BenchmarkCache_PeekBranch(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		b.Run(fmt.Sprintf("depth=%d/elements=%d", depth, depth*chainsNum+1), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nodeIdx := i % len(leaves)
				key := leaves[nodeIdx]
				if branch := cache.PeekBranch(key); len(branch) != depth {
					b.Fatalf("branch length %d, expected %d", len(branch), depth)
				}
			}
		})
	}
}

func BenchmarkCache_Add(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			// Create a cache with large enough capacity to avoid evictions
			cache, parentNodes := generateTreeForBench(b, depth, chainsNum, (depth*chainsNum+1)+b.N)

			initialSize := cache.Len()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				chainIdx := i % chainsNum
				parentKey := parentNodes[chainIdx]
				newKey := fmt.Sprintf("bench-node-%d-%d", chainIdx+1, i)
				newVal := i
				if err := cache.Add(newKey, newVal, parentKey); err != nil {
					b.Fatalf("Failed to add node %s: %v", newKey, err)
				}
			}

			// Verify no evictions occurred
			if cache.Len() != initialSize+b.N {
				b.Fatalf("Unexpected cache size. Expected %d, got %d",
					initialSize+b.N, cache.Len())
			}
		})
	}
}

func BenchmarkCache_Get_Concurrent(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	goroutineCounts := []int{32, 64, 128}
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		for _, numGoroutines := range goroutineCounts {
			b.Run(fmt.Sprintf("depth=%d/goroutines=%d", depth, numGoroutines), func(b *testing.B) {
				opsPerGoroutine := b.N / numGoroutines
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				b.ResetTimer()
				for g := 0; g < numGoroutines; g++ {
					go func(goroutineID int) {
						defer wg.Done()
						for i := 0; i < opsPerGoroutine; i++ {
							nodeIdx := (goroutineID*opsPerGoroutine + i) % len(leaves)
							key := leaves[nodeIdx]
							if _, found := cache.Get(key); !found {
								// Using panic instead of b.Fatalf because b.Fatalf isn't goroutine-safe
								panic(fmt.Sprintf("key %s not found in cache", key))
							}
						}
					}(g)
				}
				wg.Wait()
			})
		}
	}
}

func BenchmarkCache_Peek_Concurrent(b *testing.B) {
	const chainsNum = 10_000
	depths := []int{5, 10, 50} // root is the 1st level
	goroutineCounts := []int{32, 64, 128}
	for _, depth := range depths {
		cache, leaves := generateTreeForBench(b, depth, chainsNum, 0)
		for _, numGoroutines := range goroutineCounts {
			b.Run(fmt.Sprintf("depth=%d/goroutines=%d", depth, numGoroutines), func(b *testing.B) {
				opsPerGoroutine := b.N / numGoroutines
				var wg sync.WaitGroup
				wg.Add(numGoroutines)
				b.ResetTimer()
				for g := 0; g < numGoroutines; g++ {
					go func(goroutineID int) {
						defer wg.Done()
						for i := 0; i < opsPerGoroutine; i++ {
							nodeIdx := (goroutineID*opsPerGoroutine + i) % len(leaves)
							key := leaves[nodeIdx]
							if _, found := cache.Peek(key); !found {
								// Using panic instead of b.Fatalf because b.Fatalf isn't goroutine-safe
								panic(fmt.Sprintf("key %s not found in cache", key))
							}
						}
					}(g)
				}
				wg.Wait()
			})
		}
	}
}

// generateTreeForBench creates a tree with multiple linear chains from a common root.
//
// Tree structure:
// This function generates a tree with multiple parallel branches (chains) all starting from
// the common root. Each branch is a straight line until the maximum depth is reached.
// The function returns only the leaf nodes at the maximum depth level.
//
// ASCII example for maxDepth=4 and chainsNum=3:
//
//	       root
//	      /  |  \
//	    /    |    \
//	n-1-1  n-2-1  n-3-1    (depth 2)
//	  |      |      |
//	n-1-2  n-2-2  n-3-2    (depth 3)
//	  |      |      |
//	n-1-3  n-2-3  n-3-3    (depth 4, leaves)
//
// Where n-x-y means node-chainID-depth
// The return value would be the slice ["n-1-3", "n-2-3", "n-3-3"]
//
// This structure is specifically designed to test the cache's performance for
// retrieving nodes at various depths, with a focus on measuring ancestor traversal.
//
// Parameters:
//   - maxDepth: The maximum depth of the tree (including root)
//   - chainsNum: The number of parallel chains to create
//   - maxEntries: The maximum capacity of the cache. If 0, defaults to (maxDepth*chainsNum + 1)
//
// Returns:
//   - The initialized cache
//   - A slice containing the leaf node keys (nodes at maxDepth-1)
func generateTreeForBench(b *testing.B, maxDepth int, chainsNum int, maxEntries int) (*Cache[string, int], []string) {
	b.Helper()

	if maxEntries == 0 {
		maxEntries = maxDepth*chainsNum + 1
	}
	cache := NewCache[string, int](maxEntries)
	rootKey := "root"
	if err := cache.AddRoot(rootKey, 0); err != nil {
		b.Fatal(err)
	}
	leaves := make([]string, 0, chainsNum)
	for chainIdx := 0; chainIdx < chainsNum; chainIdx++ {
		parentKey := rootKey
		for depth := 1; depth < maxDepth; depth++ {
			nodeKey := fmt.Sprintf("node-%d-%d", chainIdx+1, depth)
			nodeVal := chainIdx*maxDepth + depth
			if err := cache.Add(nodeKey, nodeVal, parentKey); err != nil {
				b.Fatal(err)
			}
			if depth == maxDepth-1 {
				leaves = append(leaves, nodeKey)
			}
			parentKey = nodeKey
		}
	}
	return cache, leaves
}
