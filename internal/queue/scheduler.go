package queue

import (
	"sync"
	"time"
)

type Options struct {
	MaxParallel   int
	RetryInterval time.Duration
}

type jobState struct {
	queued  bool
	running bool
	dirty   bool
}

type Scheduler struct {
	mu          sync.Mutex
	maxParallel int
	retryAfter  time.Duration
	active      int
	order       []string
	retries     map[string]struct{}
	jobs        map[string]*jobState
}

func New(opts Options) *Scheduler {
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 1
	}
	return &Scheduler{
		maxParallel: opts.MaxParallel,
		retryAfter:  opts.RetryInterval,
		retries:     map[string]struct{}{},
		jobs:        map[string]*jobState{},
	}
}

func (s *Scheduler) MarkDirty(job string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	state.dirty = true
	if !state.queued && !state.running {
		state.queued = true
		s.order = append(s.order, job)
	}
}

func (s *Scheduler) Ready() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.order))
	for _, job := range s.order {
		if state := s.jobs[job]; state != nil && state.queued && !state.running {
			out = append(out, job)
		}
	}
	return out
}

func (s *Scheduler) TryStart(job string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if s.active >= s.maxParallel || state.running || !state.queued {
		return false
	}

	state.queued = false
	state.running = true
	state.dirty = false
	delete(s.retries, job)
	s.active++
	s.remove(job)
	return true
}

func (s *Scheduler) Finish(job string, dirty bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if state.running && s.active > 0 {
		s.active--
	}
	state.running = false
	if dirty || state.dirty {
		state.dirty = true
		if !state.queued {
			state.queued = true
			s.order = append(s.order, job)
		}
	}
}

func (s *Scheduler) FinishFailed(job string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensure(job)
	if state.running && s.active > 0 {
		s.active--
	}
	state.running = false
	s.retries[job] = struct{}{}
}

func (s *Scheduler) RetryReady() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.retries))
	for job := range s.retries {
		out = append(out, job)
	}
	return out
}

func (s *Scheduler) RetryAfter() time.Duration {
	return s.retryAfter
}

func (s *Scheduler) ensure(job string) *jobState {
	if state, ok := s.jobs[job]; ok {
		return state
	}
	state := &jobState{}
	s.jobs[job] = state
	return state
}

func (s *Scheduler) remove(job string) {
	dst := s.order[:0]
	for _, item := range s.order {
		if item != job {
			dst = append(dst, item)
		}
	}
	s.order = dst
}
