// Package cache contains a generic caching tool to be used with multiple concurrent goroutines
// that all may need to access data at the same time.
// if the data is not in the cache, the first goroutine gets informed and is allowed to enter it while the other's wait
package cache

import (
	"errors"
	"fmt"
	"sync"
)

// ErrKeyAlreadyExists when adding a key that is already in the cache
var ErrKeyAlreadyExists = errors.New("key already exists")

// ErrKeyDoesNotExist when getting a key that does not exist
var ErrKeyDoesNotExist = errors.New("key does not exists")

type AddFunc[T any] func(T, error) error

type Cache[T any] struct {
	cache map[string]T
	cm    sync.RWMutex
	locks map[string]chan error
	lm    sync.Mutex
}

func New[T any]() *Cache[T] {
	var c Cache[T]
	c.cache = make(map[string]T)
	c.locks = make(map[string]chan error)
	return &c
}

// AddCheck creates and locks the cache for the provided key, the returned function unlocks it when a value is added
// if AddFunc is never called the lock is held forever
// bool returns true if the calling thread wins the addFunc race and is responsible for adding, if false then call GetWait after to get the resulting value
func (c *Cache[T]) AddCheck(key string) (AddFunc[T], bool) {
	// get channel map lock
	c.lm.Lock()
	// test if channel already exists
	if _, ok := c.locks[key]; ok {
		// this routine lost the race condition, signal to calling thread to call GetWait
		// return nil, fmt.Errorf("unsupported add: key already exists for %q", key)
		c.lm.Unlock()
		return nil, false
	}
	// create channel
	c.locks[key] = make(chan error, 1)
	// lock channel
	// -- (locked/blocked by default)
	// release channel map mutex
	c.lm.Unlock()
	// create return function to perform real add
	f := func(value T, err error) error {
		// check if there was an error
		if err != nil {
			c.locks[key] <- err
			// this puts the channel lock in an unrecoverable state if the error if recoverable
		}
		// perform add
		c.cm.Lock()
		c.cache[key] = value
		c.cm.Unlock()
		// unlock channel
		close(c.locks[key])
		return nil
	}
	return f, true
}

// Add same as AddCheck, but adds the values immediately
func (c *Cache[T]) Add(key string, value T) error {
	f, notExists := c.AddCheck(key)
	if !notExists {
		return fmt.Errorf("unsupported add: %w for %q", ErrKeyAlreadyExists, key)
	}
	return f(value, nil)
}

// Get returns the cached data for the key or its default type if none
func (c *Cache[T]) Get(key string) (T, bool) {
	c.cm.RLock()
	value, ok := c.cache[key]
	c.cm.RUnlock()
	return value, ok
}

// GetWait returns the value for the provided key, if it does not exist yet and another worker is
// holding the lock for it this method waits until it finishes and returns the value
// if no other worker is getting the value then it returns an error
func (c *Cache[T]) GetWait(key string) (T, error) {
	// get the channel for the key
	_, ok := c.locks[key]
	var zero T // because we can't return nil with generics
	if !ok {
		return zero, fmt.Errorf("%w for channel %q", ErrKeyDoesNotExist, key)
	}
	// wait for it to return an error or close
	err := <-c.locks[key]
	// return error if error
	if err != nil {
		return zero, err
	}
	// get and return value from cache otherwise
	c.cm.RLock()
	defer c.cm.RUnlock()
	return c.cache[key], nil
}
