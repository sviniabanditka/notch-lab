package transcript

// reducer folds records into the active branch. Records come in file order,
// earlier parts before the tail; the state is the records in that order and
// the nodes built from them. Node IDs are stable, so a node's Rev says
// whether it changed since a Conversation was handed out.
type reducer struct {
	recs map[string]*rec
	// order is every record with a uuid, in file order.
	order []*rec
	// nodes hold each item's latest state by ID.
	nodes map[string]*node
	// results and outputs wait for their call or command to show up.
	results map[string]*result
	outputs map[string]string
	// hi and lo number the tail's records up from 1 and the earlier parts
	// down from 0, so that a seq orders every record of the file.
	hi, lo int
	meta   header
}

// node is an item with what the reducer needs to change it.
type node struct {
	item Item
	seq  int
	task string
}

func newReducer() *reducer {
	return &reducer{recs: map[string]*rec{}, nodes: map[string]*node{},
		results: map[string]*result{}, outputs: map[string]string{}}
}

// add applies a batch of records read in file order: the tail's as they
// come, or an earlier part's, which leaves the tip where it is.
func (r *reducer) add(batch []*rec, earlier bool) {
	base := r.lo - len(batch)
	var part []*rec
	for i, rc := range batch {
		seq := base + i
		if !earlier {
			r.hi++
			seq = r.hi
		}
		if r.apply(rc, seq, earlier) {
			part = append(part, rc)
		}
	}
	if earlier {
		r.lo = base
		r.order = append(part, r.order...)
	} else {
		r.order = append(r.order, part...)
	}
}

// apply folds one record in and reports whether it is new.
func (r *reducer) apply(rc *rec, seq int, earlier bool) bool {
	r.meta.take(rc.meta, !earlier)
	if rc.uuid == "" {
		return false
	}
	if _, dup := r.recs[rc.uuid]; dup {
		return false
	}
	r.recs[rc.uuid] = rc
	for _, res := range rc.results {
		r.result(res)
	}
	for i := range rc.items {
		r.create(&rc.items[i], rc, seq, earlier)
	}
	if rc.isOutput {
		r.output(rc)
	}
	if rc.interrupt {
		r.interrupt(seq)
	}
	return true
}

// create adds the item of a record, or merges an answer or thinking block
// into the one of its message: after it from the tail, before it from an
// earlier part.
func (r *reducer) create(it *Item, rc *rec, seq int, earlier bool) {
	n := r.nodes[it.ID]
	switch {
	case n != nil && (it.Kind == KindAnswer || it.Kind == KindThinking):
		if it.Text != "" {
			switch {
			case n.item.Text == "":
				n.item.Text = it.Text
			case earlier:
				n.item.Text = it.Text + "\n\n" + n.item.Text
			default:
				n.item.Text += "\n\n" + it.Text
			}
			n.item.Rev++
		}
		return
	case n != nil:
		return
	}
	nd := &node{item: *it, seq: seq, task: rc.task}
	nd.item.Rev, nd.item.At = 1, rc.at
	switch it.Kind {
	case KindTool:
		t := *it.Tool
		if res := r.results[it.ID]; res != nil {
			settle(&t, res)
		}
		nd.item.Tool = &t
	case KindSlashCommand:
		c := *it.Command
		if out, ok := r.outputs[rc.uuid]; ok {
			c.Output = out
			delete(r.outputs, rc.uuid)
		}
		nd.item.Command = &c
	}
	r.nodes[it.ID] = nd
}

// result pairs a tool result with its call, or parks it until the call
// comes, as a call before the tail is read later.
func (r *reducer) result(res *result) {
	r.results[res.callID] = res
	n := r.nodes[res.callID]
	if n == nil || n.item.Tool == nil {
		return
	}
	t := *n.item.Tool
	settle(&t, res)
	n.item.Tool = &t
	n.item.Rev++
}

// settle sets a call's state and what its result carries.
func settle(t *Tool, res *result) {
	t.Result, t.Images, t.Diff = res.text, res.images, res.diff
	switch {
	case res.interrupted:
		t.State = ToolInterrupted
	case res.isError:
		t.State = ToolFailed
	default:
		t.State = ToolDone
	}
}

// output gives a command the output that records after it name it as
// their parent, or parks it until the command comes.
func (r *reducer) output(rc *rec) {
	n := r.nodes[rc.parent]
	if n == nil {
		r.outputs[rc.parent] = rc.output
		return
	}
	if n.item.Command == nil || n.item.Command.Output != "" {
		return
	}
	c := *n.item.Command
	c.Output = rc.output
	n.item.Command = &c
	n.item.Rev++
}

// interrupt ends the calls still running before the marker at seq.
func (r *reducer) interrupt(seq int) {
	for _, n := range r.nodes {
		if n.item.Tool == nil || n.item.Tool.State != ToolRunning || n.seq > seq {
			continue
		}
		t := *n.item.Tool
		t.State = ToolInterrupted
		n.item.Tool = &t
		n.item.Rev++
	}
}

// items is the active branch, each item once. A notice of a task already
// shown since the last finished turn is left out: Claude writes one
// delivery both as an attachment and as a user row.
func (r *reducer) items() []Item {
	walk := r.walk()
	dropped := abandoned(walk)
	out := make([]Item, 0, len(r.nodes))
	shown := map[string]bool{}
	tasks := map[string]bool{}
	for i, rc := range walk {
		if dropped[i] {
			continue
		}
		for _, it := range rc.items {
			n := r.nodes[it.ID]
			if n == nil || shown[it.ID] {
				continue
			}
			if n.task != "" {
				if tasks[n.task] {
					continue
				}
				tasks[n.task] = true
			}
			if it.Kind == KindTurnEnd {
				clear(tasks)
			}
			shown[it.ID] = true
			out = append(out, n.item)
		}
	}
	return out
}

// walk orders the records parent before child, depth first, siblings and
// roots in file order. File order alone is not causal: Claude can write a
// root row after its children. Records whose parent is not read (before
// the tail, or none, as a compact boundary) are roots.
func (r *reducer) walk() []*rec {
	kids := make(map[string][]*rec, len(r.order))
	var roots []*rec
	for _, rc := range r.order {
		if _, ok := r.recs[rc.parent]; ok && rc.parent != rc.uuid {
			kids[rc.parent] = append(kids[rc.parent], rc)
		} else {
			roots = append(roots, rc)
		}
	}
	out := make([]*rec, 0, len(r.order))
	seen := make(map[string]bool, len(r.order))
	stack := make([]*rec, 0, len(roots))
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, roots[i])
	}
	for len(stack) > 0 {
		rc := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[rc.uuid] {
			continue
		}
		seen[rc.uuid] = true
		out = append(out, rc)
		k := kids[rc.uuid]
		for i := len(k) - 1; i >= 0; i-- {
			stack = append(stack, k[i])
		}
	}
	return out
}

// abandoned marks the records of walk a rewind left behind. A prompt or
// command whose parent P already led to another prompt or command started
// over from P, so what the walk puts between P and it is a dead branch.
// Following parentUuid back from one leaf instead would lose rows Claude
// hangs on an ancestor: a tool result after a hook attachment, parallel
// results.
func abandoned(walk []*rec) []bool {
	at := make(map[string]int, len(walk))
	// starts[i] counts the prompts and commands before index i.
	starts := make([]int, len(walk)+1)
	for i, rc := range walk {
		at[rc.uuid] = i
		starts[i+1] = starts[i]
		if rc.rewinds {
			starts[i+1]++
		}
	}
	// cut[i] > 0 while index i lies inside a dead branch.
	cut := make([]int, len(walk)+1)
	for i, rc := range walk {
		j, ok := at[rc.parent]
		if !rc.rewinds || !ok || j >= i || starts[i]-starts[j+1] == 0 {
			continue
		}
		cut[j+1]++
		cut[i]--
	}
	dropped := make([]bool, len(walk))
	for i, depth := 0, 0; i < len(walk); i++ {
		depth += cut[i]
		dropped[i] = depth > 0
	}
	return dropped
}

// take fills in the parts of m that the record carries: from the tail a
// newer record overrides, from an earlier part only what is still empty.
func (m *header) take(h header, fromTail bool) {
	fill := func(dst *string, src string) {
		if src != "" && (fromTail || *dst == "") {
			*dst = src
		}
	}
	fill(&m.version, h.version)
	fill(&m.model, h.model)
	fill(&m.cwd, h.cwd)
	fill(&m.branch, h.branch)
	fill(&m.title, h.title)
}
