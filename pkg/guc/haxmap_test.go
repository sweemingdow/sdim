package guc

import (
	"github.com/alphadose/haxmap"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkParallelHaxmap(b *testing.B) {
	m := haxmap.New[int, int](100000)
	var counter atomic.Uint64
	b.RunParallel(func(pb *testing.PB) {

		for pb.Next() {
			v := int(counter.Add(1))
			m.Set(v, v)
		}
	})
}

func BenchmarkParallelMapWithRw(b *testing.B) {
	var (
		counter atomic.Uint64
		m       = make(map[int]int, 100000)
		rw      sync.RWMutex
	)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := int(counter.Add(1))
			rw.Lock()
			m[v] = v
			rw.Unlock()
		}
	})
}

func BenchmarkHaxmapMixed(b *testing.B) {
	const capacity = 100000
	m := haxmap.New[int, int](capacity)
	// 预热：先写入 10 万个 key
	for i := 0; i < capacity; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var localCounter uint64
		for pb.Next() {
			localCounter++
			key := int(localCounter % capacity)
			// 70% 读，30% 写
			if localCounter%10 < 3 {
				m.Set(key, int(localCounter))
			} else {
				_, _ = m.Get(key)
			}
		}
	})
}

func BenchmarkMapRwMixed(b *testing.B) {
	const capacity = 100000
	m := make(map[int]int, capacity)
	var rw sync.RWMutex
	for i := 0; i < capacity; i++ {
		m[i] = i
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var localCounter uint64
		for pb.Next() {
			localCounter++
			key := int(localCounter % capacity)
			if localCounter%10 < 3 {
				rw.Lock()
				m[key] = int(localCounter)
				rw.Unlock()
			} else {
				rw.RLock()
				_ = m[key]
				rw.RUnlock()
			}
		}
	})
}
