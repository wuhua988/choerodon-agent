package manager

import "sync"

type Namespaces struct {
	m map[string]bool
	sync.RWMutex
}

func NewNamespaces() *Namespaces {
	return &Namespaces{
		m: map[string]bool{},
	}
}

func (nsSet *Namespaces) Add (ns string)  {
	nsSet.Lock()
	defer nsSet.Unlock()
	nsSet.m[ns] = true
}

func (nsSet *Namespaces) Remove (ns string)  {
	nsSet.Lock()
	defer nsSet.Unlock()
	delete(nsSet.m, ns)
}

func (nsSet *Namespaces) Contain (ns string)  bool {
	nsSet.RLock()
	defer nsSet.RUnlock()
	_, ok := nsSet.m[ns]
	return ok
}

func (nsSet *Namespaces) AddAll(nsList []string)  {
	nsSet.Lock()
	defer nsSet.Unlock()
	for _,ns := range nsList {
		nsSet.m[ns] = true
	}
}

func (nsSet *Namespaces) getAll(nsList []string)  {
	nsSet.Lock()
	defer nsSet.Unlock()
	for _,ns := range nsList {
		nsSet.m[ns] = true
	}
}