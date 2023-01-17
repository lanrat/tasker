// Package memory is used to ensure multiple identical jobs are not re-run
// its used to break curricular dependencies in parallel workflows
package memory

import (
	"log"
	"sync"
)

var DEBUG = false

type ClassType string
type KeyType string

type Mem struct {
	data map[ClassType]*memClass
	lock sync.RWMutex
}

type memClass struct {
	memory map[KeyType]bool
	lock   sync.RWMutex
}

func Memory(classes ...ClassType) *Mem {
	var m Mem
	m.data = make(map[ClassType]*memClass)
	for _, class := range classes {
		m.addClass(class)
	}
	return &m
}

func (m *Mem) addClass(class ClassType) *memClass {
	v("addClass(%s)", class)
	m.lock.Lock()
	defer m.lock.Unlock()
	c := memClass{
		memory: make(map[KeyType]bool),
	}
	m.data[class] = &c
	return &c
}

// LookupAndSave same as Save, but returns prior status as well
func (m *Mem) LookupAndSave(class ClassType, key KeyType) bool {
	v("LookupAndSave(%s, %s)", class, key)
	m.lock.RLock()
	c, ok := m.data[class]
	if !ok {
		m.lock.RUnlock()
		c = m.addClass(class)
		m.lock.RLock()
	}
	defer m.lock.RUnlock()
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.memory[key] {
		return true
	}
	c.memory[key] = true
	return false
}

func (m *Mem) Lookup(class ClassType, key KeyType) bool {
	v("Lookup(%s, %s)", class, key)
	m.lock.RLock()
	defer m.lock.RUnlock()
	if c, ok := m.data[class]; ok {
		c.lock.RLock()
		defer c.lock.RUnlock()
		return c.memory[key]
	}
	// err := fmt.Errorf("Lookup called for wrong class %s", class)
	// panic(err)
	return false
}

func (m *Mem) Save(class ClassType, key KeyType) {
	v("Save(%s, %s)", class, key)
	m.lock.RLock()
	c, ok := m.data[class]
	if !ok {
		m.lock.RUnlock()
		c = m.addClass(class)
		m.lock.RLock()
	}
	defer m.lock.RUnlock()
	c.lock.Lock()
	defer c.lock.Unlock()
	c.memory[key] = true
}

func (m *Mem) Forget(class ClassType, key KeyType) {
	v("Forget(%s, %s)", class, key)
	m.lock.RLock()
	defer m.lock.RUnlock()
	c, ok := m.data[class]
	if !ok {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.memory, key)
}

func (m *Mem) Size() int {
	i := 0
	m.lock.RLock()
	for _, c := range m.data {
		c.lock.RLock()
		i += len(c.memory)
		c.lock.RUnlock()
	}
	defer m.lock.RUnlock()
	return i
}

func v(format string, a ...any) {
	if DEBUG {
		log.Printf(format, a...)
	}
}
