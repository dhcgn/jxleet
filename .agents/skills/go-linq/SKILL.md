---
name: go-linq
description: Use github.com/ahmetb/go-linq/v5 for LINQ-style queries in Go 1.27+. Provides fully type-safe, generic-method-based query chains over slices, maps, channels, strings, and iter.Seq[T].
license: MIT
compatibility: Go 1.27+ (generic methods required). Intended for Go codebases using github.com/ahmetb/go-linq/v5.
metadata:
  version: "1.0"
  source: https://github.com/ahmetb/go-linq
---

# Skill: go-linq v5 (github.com/ahmetb/go-linq/v5)

## Purpose

Provide LINQ-style query capabilities in Go using **fully type-safe, generic-method-based** APIs introduced in **Go 1.27**.  
Use this skill whenever you need composable, lazy, type-safe queries over slices, maps, channels, strings, or `iter.Seq[T]`.

## When to Use

- Go version: **1.27+** (required for generic methods).  
- Requirement: LINQ-like query chains with **no `interface{}`/`any`**, no type assertions, and minimal allocations.  
- Import path: `github.com/ahmetb/go-linq/v5`.

> If Go < 1.27 or generic methods are unavailable, fall back to `github.com/ahmetb/go-linq/v4` (any-based API).

## Installation

```bash
go get github.com/ahmetb/go-linq/v5
```

In `go.mod`:

```go
require github.com/ahmetb/go-linq/v5 v5.0.0
```

## Core Concepts

- `Query[T]` is the central type; all operators preserve or transform `T` in a type-safe way.
- Type-changing operators (e.g. `Select`, `SelectMany`, `Join`, `GroupBy`) are **generic methods** whose result type is inferred from your functions.
- No reflection-based `…T` variants: base methods are already typed.
- Terminals like `First`, `Last`, `Single`, `Min`, `Max` return `(T, bool)` instead of `any`.
- `ToSlice()` returns `[]T` directly (no pointer argument).
- Data sources use typed constructors: `FromSlice`, `FromMap`, `FromChannel`, `FromString`, `FromSeq`, `Range`, `Repeat`.

## Usage Examples

### Example 1: Simple filter + projection (slice of structs)

```go
import . "github.com/ahmetb/go-linq/v5"

type Car struct {
    Year  int
    Owner string
    Model string
}

func ownersAfter2015(cars []Car) []string {
    return FromSlice(cars).
        Where(func(c Car) bool { return c.Year >= 2015 }).
        Select(func(c Car) string { return c.Owner }).
        ToSlice()
}
```

### Example 2: Find author with most books (grouping + aggregation)

```go
import . "github.com/ahmetb/go-linq/v5"

type Book struct {
    ID      int
    Title   string
    Authors []string
}

func topAuthor(books []Book) (author string, ok bool) {
    g, ok := FromSlice(books).
        SelectMany(func(b Book) Query[string] {
            return FromSlice(b.Authors)
        }).
        GroupBy(
            func(a string) string { return a },
            func(a string) string { return a },
        ).
        MaxBy(func(g Group[string, string]) int { return len(g.Group) })

    if !ok {
        return "", false
    }
    return g.Key, true
}
```

### Example 3: MapReduce-style word frequency (top 5 words)

```go
import . "github.com/ahmetb/go-linq/v5"

func top5Words(sentences []string) []string {
    return FromSlice(sentences).
        SelectMany(func(s string) Query[string] {
            return FromSlice(strings.Split(s, " "))
        }).
        GroupBy(
            func(w string) string { return w },
            func(w string) string { return w },
        ).
        OrderByDescending(func(g Group[string, string]) int { return len(g.Group) }).
        ThenBy(func(g Group[string, string]) string { return g.Key }).
        Take(5).
        SelectIndexed(func(i int, g Group[string, string]) string {
            return fmt.Sprintf("Rank: #%d, Word: %s, Count: %d", i+1, g.Key, len(g.Group))
        }).
        ToSlice()
}
```

### Example 4: Custom extension method

```go
import . "github.com/ahmetb/go-linq/v5"

type MyQuery Query[int]

func (q MyQuery) GreaterThan(threshold int) Query[int] {
    return Query[int](q).Where(func(v int) bool { return v > threshold })
}

func gt5() []int {
    return MyQuery(Range(1, 10)).GreaterThan(5).ToSlice()
}
```

### Example 5: Iteration with standard `for range`

```go
import . "github.com/ahmetb/go-linq/v5"

q := FromSlice([]int{1, 2, 3, 4})

for v := range q.Iterate {
    fmt.Println(v)
}
```

## Common Patterns

- Filtering + projection: `FromSlice(xs).Where(...).Select(...).ToSlice()`
- Flattening: `SelectMany` to turn `[]T` into `Query[U]`.
- Grouping: `GroupBy(keyFunc, valueFunc)` → `Query[Group[K, V]]`.
- Aggregation: use `MaxBy`, `MinBy`, `SumBy`, `AverageBy` for chainable aggregations; or package-level `Max`, `Min`, `Sum`, etc.
- Ordering: `OrderBy` / `OrderByDescending` / `ThenBy` with `cmp.Ordered` keys; use `Sort` for custom comparators.

## Migration Notes (v4 → v5)

- Replace `Query` with `Query[T]` implicitly via typed constructors (`FromSlice`, etc.).
- Remove all `…T` methods (`WhereT`, `SelectT`, …); use `Where`, `Select`, etc. directly.
- Replace `From(any)` with `FromSlice`, `FromMap`, `FromChannel`, `FromString`, `FromSeq`.
- Adjust terminals: `First()`, `Last()`, etc. now return `(T, bool)`.
- `ToSlice()` now returns `[]T`; remove the `&slice` argument pattern.

## Performance Characteristics

- 5–15× faster than v4 `any`-based API.
- Allocations drop from O(n) per query to O(1) per query (only fixed closure captures).
- Ideal for high-throughput pipelines over large slices/iterators.

## References

- Library: https://github.com/ahmetb/go-linq (v5 branch / v5 module)  
- Docs & examples: https://pkg.go.dev/github.com/ahmetb/go-linq/v5  
- Migration guide: `MIGRATION.md` in the go-linq repo.