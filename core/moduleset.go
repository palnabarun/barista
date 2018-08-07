// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"sync"

	"github.com/soumya92/barista/bar"
)

type ModuleSet struct {
	modules   []*Module
	updateCh  chan int
	outputs   []bar.Segments
	outputsMu sync.RWMutex
}

func NewModuleSet(modules []bar.Module) *ModuleSet {
	set := &ModuleSet{
		modules:  make([]*Module, len(modules)),
		outputs:  make([]bar.Segments, len(modules)),
		updateCh: make(chan int),
	}
	for i, m := range modules {
		set.modules[i] = NewModule(m)
	}
	return set
}

func (set *ModuleSet) Stream() <-chan int {
	for i, m := range set.modules {
		go m.Stream(set.sinkFn(i))
	}
	return set.updateCh
}

func (m *ModuleSet) sinkFn(idx int) Sink {
	return func(out bar.Segments) {
		m.outputsMu.Lock()
		m.outputs[idx] = out
		m.outputsMu.Unlock()
		m.updateCh <- idx
	}
}

func (m *ModuleSet) Click(idx int, e bar.Event) {
	m.modules[idx].Click(e)
}

func (m *ModuleSet) LastOutput(idx int) bar.Segments {
	m.outputsMu.RLock()
	defer m.outputsMu.RUnlock()
	return m.outputs[idx]
}

func (m *ModuleSet) LastOutputs() []bar.Segments {
	m.outputsMu.RLock()
	defer m.outputsMu.RUnlock()
	cp := make([]bar.Segments, len(m.outputs))
	copy(cp, m.outputs)
	return cp
}
