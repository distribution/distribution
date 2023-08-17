// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fsm

import (
	"fmt"
	"io"
)

// DumpFSM accepts a io.writer and write the current FSM into dot file format.
func (f *FSM) DumpFSM(w io.Writer) {
	idx := 0
	states := make(map[int]*mappingState)
	states[idx] = f.root

	w.Write([]byte("digraph g {\n"))
	w.Write([]byte("rankdir=LR\n"))                                                    // make it vertical
	w.Write([]byte("node [ label=\"\",style=filled,fillcolor=white,shape=circle ]\n")) // remove label of node

	for idx < len(states) {
		for field, transition := range states[idx].transitions {
			states[len(states)] = transition
			w.Write([]byte(fmt.Sprintf("%d -> %d  [label = \"%s\"];\n", idx, len(states)-1, field)))
			if idx == 0 {
				// color for metric types
				w.Write([]byte(fmt.Sprintf("%d [color=\"#D6B656\",fillcolor=\"#FFF2CC\"];\n", len(states)-1)))
			} else if transition.transitions == nil || len(transition.transitions) == 0 {
				// color for end state
				w.Write([]byte(fmt.Sprintf("%d [color=\"#82B366\",fillcolor=\"#D5E8D4\"];\n", len(states)-1)))
			}
		}
		idx++
	}
	// color for start state
	w.Write([]byte(fmt.Sprintf("0 [color=\"#a94442\",fillcolor=\"#f2dede\"];\n")))
	w.Write([]byte("}"))
}
