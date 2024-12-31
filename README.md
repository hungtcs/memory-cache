# Memory Cache

This is a simple memory cache manager.

It achieves thread safety through `sync.RWMutex` and automatically cleans up expired data using `time.Ticker`.

## Features

1. Thread-safe
2. Support generics
3. Expiration and auto cleanup
4. Lightweight

## API

```go
// public functions
func NewMemoryCache[T any](optFuncs ...CacheOptionsFunc) *MemoryCache[T]

// option functions
func WithExpiration(expiration time.Duration) CacheOptionsFunc // global and peer item
func WithMaxSize(maxSize int) CacheOptionsFunc // global only
func WithLogger(log Logger) CacheOptionsFunc // global only
func WithCleanupInterval(interval time.Duration) CacheOptionsFunc // global only

// cache manager methods
Get(key string) (_ *T, ok bool)
Take(key string) (_ *T, ok bool)
Set(key string, value T, optionFunc ...CacheOptionsFunc) (err error)
Update(key string, updater func(value T) T, optionFunc ...CacheOptionsFunc) (ok bool)
Upsert(key string, updater func(value *T) *T, optionFunc ...CacheOptionsFunc) (err error)
Has(key string) (ok bool)
Delete(key string)
Clear()
Size() int
IsEmpty() bool
Keys() []string
Iterator() iter.Seq2[string, T] // for k, v := range cache.Iterator()
```

## Examples

```go
package main

import (
  "fmt"
  "log"
  "os"
  "time"

  memory_cache "github.com/hungtcs/memory-cache"
)

func main() {
  cache := memory_cache.NewMemoryCache[string](
    memory_cache.WithLogger(log.New(os.Stdout, "[MEMORY_CACHE] ", log.Ldate|log.Ltime|log.Lmsgprefix)), // set logger
    memory_cache.WithMaxSize(3),                   // may cause ErrCacheFull error
    memory_cache.WithCleanupInterval(time.Second), // set cleanup interval
    memory_cache.WithExpiration(time.Second*3),    // set default expiration
  )
  defer cache.Close()

  cache.Set("1", "a")
  cache.Set("2", "b")
  cache.Set("3", "c", memory_cache.WithExpiration(time.Second*10))
  cache.Set("4", "d") // error cache is full

  time.Sleep(time.Second * 5) // 1 and 2 is expired

  // only 3 is available
  fmt.Println("cache size: ", cache.Size())
  for key, val := range cache.Iterator() {
    fmt.Printf("key=%q value=%q\n", key, val)
  }
}
```

You can run this example on Go Playground <https://goplay.tools/snippet/RMsIhR99d6y>.
