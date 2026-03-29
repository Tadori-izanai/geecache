package singleflight

import "sync"

// call is ongoing or completed requests.
type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

// Group manages requests (calls) of different keys.
type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}

	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	// Non-critical begin
	c.val, c.err = fn()
	c.wg.Done()
	// Non-critical end

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

// 注：在并发场景下（获取同一个 key）
// - 第一个获得锁的线程（称为 T0）会创建一个新的 call，加入 map，然后释放锁，开始 fn() 的计算
// - 计算过程中，其他线程（称为 T1, T2, ...）获得锁、通过 map 获取 call、释放锁、阻塞等待 fn() 计算结果
// - 等到 fn() 计算完成，T1, T2, ... 这些线程将被唤醒；与此同时，T0 可以获得锁，然后在 map 中删除 key 和 对应的 call
// - 即使 T0 把 key, call 在 map 中删除了，T1, T2, ... 这些线程还能有效地获取 c.val, c.err 的值
//
// todo: read https://pkg.go.dev/golang.org/x/sync/singleflight
