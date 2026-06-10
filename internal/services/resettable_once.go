package services

import "sync"

type ResettableOnce struct {
	mu   sync.Mutex
	once *sync.Once
}

func NewResettableOnce() *ResettableOnce {
	return &ResettableOnce{once: &sync.Once{}}
}

func (r *ResettableOnce) Do(f func()) {
	if f == nil || r == nil {
		return
	}

	r.mu.Lock()
	once := r.once
	r.mu.Unlock()

	once.Do(f)
}

func (r *ResettableOnce) Reset() {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.once = &sync.Once{}
}
