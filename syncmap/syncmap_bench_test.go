package syncmap_test

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/cristaloleg/go-gen-syncmap/syncmap"
)

func newKeyT(i int) KeyT {
	// PLEASE FEEL WITH A MEANINGFUL CODE
	var defaultKey KeyT
	return defaultKey + KeyT(i)
}

func newValueT(i int) ValueT {
	// PLEASE FEEL WITH A MEANINGFUL CODE
	var defaultValue ValueT
	return defaultValue
}

type bench struct {
	setup func(*testing.B, mapInterface)
	perG  func(b *testing.B, pb *testing.PB, i int, m mapInterface)
}

func benchMap(b *testing.B, bench bench) {
	for _, m := range [...]mapInterface{&DeepCopyMap{}, &RWMutexMap{}, &syncmap.Map{}} {
		b.Run(fmt.Sprintf("%T", m), func(b *testing.B) {
			m = reflect.New(reflect.TypeOf(m).Elem()).Interface().(mapInterface)
			if bench.setup != nil {
				bench.setup(b, m)
			}

			b.ResetTimer()

			var i int64
			b.RunParallel(func(pb *testing.PB) {
				id := int(atomic.AddInt64(&i, 1) - 1)
				bench.perG(b, pb, id*b.N, m)
			})
		})
	}
}

func BenchmarkLoadMostlyHits(b *testing.B) {
	const hits, misses = 1023, 1

	benchMap(b, bench{
		setup: func(_ *testing.B, m mapInterface) {
			for i := 0; i < hits; i++ {
				m.LoadOrStore(newKeyT(i), newValueT(i))
			}
			// Prime the map to get it into a steady state.
			for i := 0; i < hits*2; i++ {
				m.Load(newKeyT(i % hits))
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.Load(newKeyT(i % (hits + misses)))
			}
		},
	})
}

func BenchmarkLoadMostlyMisses(b *testing.B) {
	const hits, misses = 1, 1023

	benchMap(b, bench{
		setup: func(_ *testing.B, m mapInterface) {
			for i := 0; i < hits; i++ {
				m.LoadOrStore(newKeyT(i), newValueT(i))
			}
			// Prime the map to get it into a steady state.
			for i := 0; i < hits*2; i++ {
				m.Load(newKeyT(i % hits))
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.Load(newKeyT(i % (hits + misses)))
			}
		},
	})
}

func BenchmarkLoadOrStoreBalanced(b *testing.B) {
	const hits, misses = 128, 128

	benchMap(b, bench{
		setup: func(b *testing.B, m mapInterface) {
			if _, ok := m.(*DeepCopyMap); ok {
				b.Skip("DeepCopyMap has quadratic running time.")
			}
			for i := 0; i < hits; i++ {
				m.LoadOrStore(newKeyT(i), newValueT(i))
			}
			// Prime the map to get it into a steady state.
			for i := 0; i < hits*2; i++ {
				m.Load(newKeyT(i % hits))
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				j := i % (hits + misses)
				if j < hits {
					if _, ok := m.LoadOrStore(newKeyT(j), newValueT(i)); !ok {
						b.Fatalf("unexpected miss for %v", j)
					}
				} else {
					if v, loaded := m.LoadOrStore(newKeyT(i), newValueT(i)); loaded {
						b.Fatalf("failed to store %v: existing value %v", i, v)
					}
				}
			}
		},
	})
}

func BenchmarkLoadOrStoreUnique(b *testing.B) {
	benchMap(b, bench{
		setup: func(b *testing.B, m mapInterface) {
			if _, ok := m.(*DeepCopyMap); ok {
				b.Skip("DeepCopyMap has quadratic running time.")
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.LoadOrStore(newKeyT(i), newValueT(i))
			}
		},
	})
}

func BenchmarkLoadOrStoreCollision(b *testing.B) {
	var defaultKey KeyT
	var defaultValue ValueT

	benchMap(b, bench{
		setup: func(_ *testing.B, m mapInterface) {
			m.LoadOrStore(defaultKey, defaultValue)
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.LoadOrStore(defaultKey, defaultValue)
			}
		},
	})
}

func BenchmarkRange(b *testing.B) {
	const mapSize = 1 << 10

	benchMap(b, bench{
		setup: func(_ *testing.B, m mapInterface) {
			for i := 0; i < mapSize; i++ {
				m.Store(newKeyT(i), newValueT(i))
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.Range(func(_ KeyT, _ ValueT) bool { return true })
			}
		},
	})
}

// BenchmarkAdversarialAlloc tests performance when we store a new value
// immediately whenever the map is promoted to clean and otherwise load a
// unique, missing key.
//
// This forces the Load calls to always acquire the map's mutex.
func BenchmarkAdversarialAlloc(b *testing.B) {
	var defaultValue ValueT
	benchMap(b, bench{
		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			var stores, loadsSinceStore int64
			for ; pb.Next(); i++ {
				m.Load(newKeyT(i))
				if loadsSinceStore++; loadsSinceStore > stores {
					m.LoadOrStore(newKeyT(i), defaultValue)
					loadsSinceStore = 0
					stores++
				}
			}
		},
	})
}

// BenchmarkAdversarialDelete tests performance when we periodically delete
// one key and add a different one in a large map.
//
// This forces the Load calls to always acquire the map's mutex and periodically
// makes a full copy of the map despite changing only one entry.
func BenchmarkAdversarialDelete(b *testing.B) {
	const mapSize = 1 << 10

	benchMap(b, bench{
		setup: func(_ *testing.B, m mapInterface) {
			for i := 0; i < mapSize; i++ {
				m.Store(newKeyT(i), newValueT(i))
			}
		},

		perG: func(b *testing.B, pb *testing.PB, i int, m mapInterface) {
			for ; pb.Next(); i++ {
				m.Load(newKeyT(i))

				if i%mapSize == 0 {
					m.Range(func(k KeyT, _ ValueT) bool {
						m.Delete(k)
						return false
					})
					m.Store(newKeyT(i), newValueT(i))
				}
			}
		},
	})
}
