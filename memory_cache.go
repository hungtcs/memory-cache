package memory_cache

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCacheFull   = fmt.Errorf("cache is full")
	ErrCacheClosed = fmt.Errorf("cache is closed")
)

type Logger interface {
	Printf(format string, v ...interface{})
}

type CacheItemOptions struct {
	Expiration time.Duration // default 0, nit expire
}

type CacheOptions struct {
	Logger          Logger
	MaxSize         int           // default 0, not limit
	Expiration      time.Duration // default 0, nit expire
	CleanupInterval time.Duration // default 30s
}

type CacheOptionsFunc func(options *CacheOptions)

// Set expiration time, which can be used globally or set separately during Set
func WithExpiration(expiration time.Duration) CacheOptionsFunc {
	return func(options *CacheOptions) {
		options.Expiration = expiration
	}
}

// Set cache max size, only working with new function
func WithMaxSize(maxSize int) CacheOptionsFunc {
	return func(options *CacheOptions) {
		options.MaxSize = maxSize
	}
}

// Set logger, only working with new function
func WithLogger(log Logger) CacheOptionsFunc {
	return func(options *CacheOptions) {
		options.Logger = log
	}
}

// Set cleanup interval, only working with new function
func WithCleanupInterval(interval time.Duration) CacheOptionsFunc {
	return func(options *CacheOptions) {
		options.CleanupInterval = interval
	}
}

type CacheItem[T any] struct {
	Value     T
	ExpiresAt time.Time
}

func (c *CacheItem[T]) IsExpired() bool {
	if IsZeroValue(c.ExpiresAt) {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

type MemoryCache[T any] struct {
	ctx     context.Context
	cancel  context.CancelFunc
	closed  atomic.Bool
	logger  Logger
	mutex   sync.RWMutex
	options CacheOptions // default options
	store   map[string]*CacheItem[T]
}

// Get returns the value for the given key, or nil if not found
func (c *MemoryCache[T]) Get(key string) (_ *T, ok bool) {
	if c.closed.Load() {
		return nil, false
	}

	defer func() {
		c.logger.Printf("get key: %v, ok: %v", key, ok)
	}()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, ok := c.store[key]
	if !ok || item.IsExpired() {
		return nil, false
	}

	return Pointer(item.Value), true
}

// Take returns the value for the given key, and remove it from cache if it exists
func (c *MemoryCache[T]) Take(key string) (_ *T, ok bool) {
	if c.closed.Load() {
		return nil, false
	}

	defer func() {
		c.logger.Printf("take key: %v, ok: %v", key, ok)
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	item, ok := c.store[key]
	if !ok || item.IsExpired() {
		return nil, false
	}

	delete(c.store, key)

	return Pointer(item.Value), true
}

// Set the value for the given key
func (c *MemoryCache[T]) Set(key string, value T, optionFunc ...CacheOptionsFunc) (err error) {
	if c.closed.Load() {
		return ErrCacheClosed
	}

	defer func() {
		if err == nil {
			c.logger.Printf("set key=%q", key)
		} else {
			c.logger.Printf("set failed key=%q err=%q", key, err)
		}
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// check cache is full
	if c.options.MaxSize > 0 && len(c.store) >= c.options.MaxSize && c.store[key] == nil {
		return ErrCacheFull
	}

	options := c.options
	for _, optionFunc := range optionFunc {
		optionFunc(&options)
	}

	item := CacheItem[T]{
		Value: value,
	}
	if options.Expiration > 0 {
		item.ExpiresAt = time.Now().Add(options.Expiration)
	}

	c.store[key] = &item

	return nil
}

// Update the value for the given key, do nothing if the key does not exist
func (c *MemoryCache[T]) Update(key string, updater func(value T) T, optionFunc ...CacheOptionsFunc) (ok bool) {
	if c.closed.Load() {
		return false
	}

	defer func() {
		c.logger.Printf("update key=%q ok=%v", key, ok)
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	options := c.options
	for _, optionFunc := range optionFunc {
		optionFunc(&options)
	}

	item, ok := c.store[key]
	if !ok || item.IsExpired() {
		return false
	}

	item.Value = updater(item.Value)
	if options.Expiration > 0 {
		item.ExpiresAt = time.Now().Add(options.Expiration)
	}

	c.store[key] = item

	return true
}

// Upsert the value for the given key, create a new value if the key does not exist.
// Do nothing is updater returns nil
func (c *MemoryCache[T]) Upsert(key string, updater func(value *T) *T, optionFunc ...CacheOptionsFunc) (err error) {
	if c.closed.Load() {
		return ErrCacheClosed
	}

	defer func() {
		if err == nil {
			c.logger.Printf("upsert key=%q", key)
		} else {
			c.logger.Printf("upsert failed key=%q err=%q", key, err)
		}
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	var vp *T
	if item, ok := c.store[key]; !ok || item.IsExpired() {
		vp = updater(nil)
	} else {
		vp = updater(Pointer(item.Value))
	}

	if vp == nil {
		return nil
	}

	if c.options.MaxSize > 0 && len(c.store) >= c.options.MaxSize && c.store[key] == nil {
		return ErrCacheFull
	}

	options := c.options
	for _, optionFunc := range optionFunc {
		optionFunc(&options)
	}

	item := CacheItem[T]{
		Value: *vp,
	}
	if options.Expiration > 0 {
		item.ExpiresAt = time.Now().Add(options.Expiration)
	}

	c.store[key] = &item

	return nil
}

// Has returns true if the cache contains the given key
func (c *MemoryCache[T]) Has(key string) (ok bool) {
	if c.closed.Load() {
		return false
	}

	defer func() {
		c.logger.Printf("has key=%q ok=%v", key, ok)
	}()

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	item, ok := c.store[key]
	if !ok || item.IsExpired() {
		return false
	}
	return true
}

// Delete the value for the given key
func (c *MemoryCache[T]) Delete(key string) {
	if c.closed.Load() {
		return
	}

	defer func() {
		c.logger.Printf("delete key=%q", key)
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.store, key)
}

// Clear the cache, remove all items
func (c *MemoryCache[T]) Clear() {
	if c.closed.Load() {
		return
	}

	defer func() {
		c.logger.Printf("clear")
	}()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.store = make(map[string]*CacheItem[T])
}

// Size returns the number of items in the cache
func (c *MemoryCache[T]) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.store)
}

// IsEmpty returns true if the cache is empty
func (c *MemoryCache[T]) IsEmpty() bool {
	return c.Size() == 0
}

// Keys returns all the keys in the cache.
func (c *MemoryCache[T]) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	keys := make([]string, 0, len(c.store))
	for k, item := range c.store {
		if item.IsExpired() {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

// for go range loop
func (c *MemoryCache[T]) Iterator() iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		if c.closed.Load() {
			return
		}

		defer func() {
			c.logger.Printf("end iterate.")
		}()

		keys := c.Keys()

		c.logger.Printf("start iterate count=%v", len(keys))
		for _, key := range keys {
			if val, ok := c.Get(key); ok {
				if !yield(key, *val) {
					return
				}
			}

		}
	}
}

func (c *MemoryCache[T]) cleanup() {
	c.logger.Printf("start cleanup ticker")

	ticker := time.NewTicker(c.options.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.closed.Load() {
				return
			}
			c.logger.Printf("start cleanup...")
			err := SafeCall(func() {
				c.mutex.Lock()
				defer c.mutex.Unlock()
				for key, item := range c.store {
					if item.IsExpired() {
						c.logger.Printf("cleanup key: %v", key)
						delete(c.store, key)
					}
				}
			})
			if err != nil {
				c.logger.Printf("cleanup failed: %v", err)
			}
		}
	}
}

// stop ticker and clear all data
func (c *MemoryCache[T]) Close() {
	c.logger.Printf("close cache...")
	defer c.logger.Printf("closed")

	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.cancel()
	c.Clear()
}

// NewMemoryCache returns a new MemoryCache.
// Support options:
//   - WithLogger
//   - WithMaxSize
//   - WithExpiration
//   - WithCleanupInterval
func NewMemoryCache[T any](optFuncs ...CacheOptionsFunc) *MemoryCache[T] {
	ctx, cancel := context.WithCancel(context.Background())

	options := CacheOptions{
		Logger:          log.New(io.Discard, "", 0),
		MaxSize:         0,
		Expiration:      0,
		CleanupInterval: time.Second * 30,
	}
	for _, optFunc := range optFuncs {
		optFunc(&options)
	}

	cache := &MemoryCache[T]{
		store:   make(map[string]*CacheItem[T]),
		ctx:     ctx,
		cancel:  cancel,
		logger:  options.Logger,
		options: options,
	}

	go cache.cleanup()

	return cache
}
