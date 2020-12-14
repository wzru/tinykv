package storage

// Modify is a single modification to TinyKV's underlying storage.
type Modify struct {
	Data interface{}
}

//Put means put (key, value) into cf
type Put struct {
	Key   []byte
	Value []byte
	Cf    string
}

//Delete means delete (key, ...) from cf
type Delete struct {
	Key []byte
	Cf  string
}

//Key will return the key
func (m *Modify) Key() []byte {
	switch m.Data.(type) {
	case Put:
		return m.Data.(Put).Key
	case Delete:
		return m.Data.(Delete).Key
	}
	return nil
}

//Value will return the value
func (m *Modify) Value() []byte {
	if putData, ok := m.Data.(Put); ok {
		return putData.Value
	}
	return nil
}

//Cf will return the Cf
func (m *Modify) Cf() string {
	switch m.Data.(type) {
	case Put:
		return m.Data.(Put).Cf
	case Delete:
		return m.Data.(Delete).Cf
	}
	return ""
}
