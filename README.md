# LRU Tree Cache

[![GoDoc Widget]][GoDoc]

A hierarchical caching Go library that maintains parent-child relationships with an LRU (Least Recently Used) eviction policy.
This library is designed for efficiently caching tree-structured data while maintaining relational integrity and operating within memory constraints.

## Installation

```
go get -u github.com/vasayxtx/go-lrutree
```

## Key Features

+ **Hierarchical Structure**: Maintains parent-child relationships in a tree structure
+ **LRU Eviction Policy**: Automatically removes the least recently used leaf nodes when the maximum size is reached
+ **Memory-Constrained Caching**: Ideal for caching tree-structured data with limited memory
+ **Type Safety**: Built with Go generics for strong type safety
+ **Concurrent Access**: Thread-safe implementation
+ **Efficient Traversal**: Methods to traverse up to root or down through subtrees
+ **Integrity Guarantee**: Ensures a node's ancestors are always present in the cache

## Use Cases

LRU Tree Cache is particularly useful for:

+ Hierarchical Data Caching: Organizations, file systems, taxonomies
+ Access Control Systems: Caching permission hierarchies
+ Geo-Data: Caching location hierarchies (country -> state -> city -> district)
+ E-commerce: Product categories and subcategories

## Usage

```go
package main

import (
	"fmt"

	"github.com/vasayxtx/go-lrutree"
)

type OrgItem struct {
	Name string
}

func main() {
	// Create a new cache with a maximum size of 4 entries and an eviction callback.
	cache := lrutree.NewCache[string, OrgItem](4, lrutree.WithOnEvict(func(node lrutree.CacheNode[string, OrgItem]) {
		fmt.Printf("Evicted: %s (key=%s, parent=%s)\n", node.Value.Name, node.Key, node.ParentKey)
	}))

	// Add nodes to the cache.
	_ = cache.AddRoot("company", OrgItem{"My Company"})
	_ = cache.Add("engineering", OrgItem{"Engineering department"}, "company")
	_ = cache.Add("frontend", OrgItem{"Frontend team"}, "engineering")
	_ = cache.Add("backend", OrgItem{"Backend team"}, "engineering")

	// Get the value by key.
	// "frontend" node and all its ancestors ("engineering" and "company" nodes) are marked as recently used.
	if cacheNode, ok := cache.Get("frontend"); ok {
		fmt.Printf("Get: %s (key=%s, parent=%s)\n", cacheNode.Value.Name, cacheNode.Key, cacheNode.ParentKey)
		// Output: Get: Frontend team (key=frontend, parent=engineering)
	}

	// Get the full branch from the root to the node with key "backend".
	// "backend", "engineering", and "company" nodes are marked as recently used.
	branch := cache.GetBranch("backend")
	for i, node := range branch {
		fmt.Printf("GetBranch[%d]: %s (key=%s, parent=%s)\n", i, node.Value.Name, node.Key, node.ParentKey)
	}
	// Output:
	// GetBranch[0]: My Company (key=company, parent=)
	// GetBranch[1]: Engineering department (key=engineering, parent=company)
	// GetBranch[2]: Backend team (key=backend, parent=engineering)

	// Peek the value by key without updating the LRU order.
	if cacheNode, ok := cache.Peek("frontend"); ok {
		fmt.Printf("Peek: %s (key=%s, parent=%s)\n", cacheNode.Value.Name, cacheNode.Key, cacheNode.ParentKey)
		// Output: Peek: Frontend team (key=frontend, parent=engineering)
	}

	// Add a new node exceeding the cache's maximum size.
	// The least recently used leaf node ("frontend") is evicted.
	_ = cache.Add("architects", OrgItem{"Architects team"}, "engineering")
	// Output: Evicted: Frontend team (key=frontend, parent=engineering)
}
```

More advanced usage example can be found in [example_test.go](./example_test.go).

## Behavior Notes

+ When a node is accessed, it and all its ancestors are marked as recently used.
+ The cache enforces a strict maximum size, automatically evicting the least recently used leaf nodes when the limit is reached.
+ When eviction occurs, only leaf nodes (nodes without children) can be removed.
+ The cache guarantees that if a node exists, all its ancestors up to the root also exist.

## Performance Considerations

+ The cache uses mutex locks for thread safety, which can impact performance under high concurrency.
+ For write-heavy workloads, consider using multiple caches with sharding.

### Benchmarking

The package includes [benchmarks](./cache_benchmark_test.go) to evaluate the performance of the LRU tree cache under various conditions.
The following results were obtained on Apple M1 Max with Go 1.20:
```
BenchmarkCache_Get/depth=5/elements=50001-10                    13252944               89.38 ns/op
BenchmarkCache_Get/depth=10/elements=100001-10                   5915546               201.7 ns/op
BenchmarkCache_Get/depth=50/elements=500001-10                    847310                1331 ns/op
BenchmarkCache_Peek/depth=5/elements=50001-10                   15514975               78.23 ns/op
BenchmarkCache_Peek/depth=10/elements=100001-10                 14090122               95.83 ns/op
BenchmarkCache_Peek/depth=50/elements=500001-10                  7754596               200.7 ns/op
BenchmarkCache_GetBranch/depth=5/elements=50001-10               7374289               161.4 ns/op
BenchmarkCache_GetBranch/depth=10/elements=100001-10             3946384               318.8 ns/op
BenchmarkCache_GetBranch/depth=50/elements=500001-10              620029                2325 ns/op
BenchmarkCache_PeekBranch/depth=5/elements=50001-10              6649334               188.6 ns/op
BenchmarkCache_PeekBranch/depth=10/elements=100001-10            3969432               305.5 ns/op
BenchmarkCache_PeekBranch/depth=50/elements=500001-10             578042                2308 ns/op
BenchmarkCache_Add/depth=5-10                                    1631062               698.9 ns/op
BenchmarkCache_Add/depth=10-10                                   1587628               734.6 ns/op
BenchmarkCache_Add/depth=50-10                                    769306                1714 ns/op

BenchmarkCache_Get_Concurrent/depth=5/goroutines=32-10           5557310               195.9 ns/op
BenchmarkCache_Get_Concurrent/depth=5/goroutines=64-10           5450746               208.9 ns/op
BenchmarkCache_Get_Concurrent/depth=5/goroutines=128-10          4809602               226.0 ns/op
BenchmarkCache_Get_Concurrent/depth=10/goroutines=32-10          4208766               288.0 ns/op
BenchmarkCache_Get_Concurrent/depth=10/goroutines=64-10          4313493               284.3 ns/op
BenchmarkCache_Get_Concurrent/depth=10/goroutines=128-10         4019263               316.7 ns/op
BenchmarkCache_Get_Concurrent/depth=50/goroutines=32-10           849674                1223 ns/op
BenchmarkCache_Get_Concurrent/depth=50/goroutines=64-10          1033851                1174 ns/op
BenchmarkCache_Get_Concurrent/depth=50/goroutines=128-10          950826                1231 ns/op
BenchmarkCache_Peek_Concurrent/depth=5/goroutines=32-10          6772576               179.3 ns/op
BenchmarkCache_Peek_Concurrent/depth=5/goroutines=64-10          6705830               177.4 ns/op
BenchmarkCache_Peek_Concurrent/depth=5/goroutines=128-10         8474424               151.9 ns/op
BenchmarkCache_Peek_Concurrent/depth=10/goroutines=32-10         7283060               142.4 ns/op
BenchmarkCache_Peek_Concurrent/depth=10/goroutines=64-10         6966217               170.5 ns/op
BenchmarkCache_Peek_Concurrent/depth=10/goroutines=128-10        6767965               150.1 ns/op
BenchmarkCache_Peek_Concurrent/depth=50/goroutines=32-10         7615438               134.0 ns/op
BenchmarkCache_Peek_Concurrent/depth=50/goroutines=64-10         9349428               158.0 ns/op
BenchmarkCache_Peek_Concurrent/depth=50/goroutines=128-10        7689594               156.4 ns/op
```

## License

MIT License - see [LICENSE](./LICENSE) file for details.

[GoDoc]: https://pkg.go.dev/github.com/vasayxtx/go-lrutree
[GoDoc Widget]: https://godoc.org/github.com/vasayxtx/go-lrutree?status.svg