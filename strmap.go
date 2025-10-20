package djson

type mapEntry struct {
	key   string
	value interface{}
}

type StrMap struct {
	keys        map[string]uint32 // key -> to index
	values      []mapEntry
	iterCounter int
}

func NewStrMap() *StrMap {
	return &StrMap{
		keys: make(map[string]uint32),
	}
}

var gDeletedEntryValue int
var gDeletedEntry interface{} = &gDeletedEntryValue

func (self *StrMap) Set(key string, value interface{}) {

	if inx, ok := self.keys[key]; ok {
		self.values[inx].value = value
		return
	}

	inx := uint32(len(self.values))
	self.keys[key] = inx
	self.values = append(self.values, mapEntry{key: key, value: value})
}

func (self *StrMap) Get(key string) (interface{}, bool) {
	if self == nil {
		return nil, false
	}

	inx, ok := self.keys[key]

	if !ok {
		return nil, false
	}

	return self.values[inx].value, true
}

func (self *StrMap) Len() int {
	if self == nil {
		return 0
	}
	return len(self.keys)
}

func (self *StrMap) Delete(key string) {

	if self == nil {
		return
	}

	inx, ok := self.keys[key]

	if !ok {
		return
	}
	self.values[inx] = mapEntry{value: gDeletedEntry}
	delete(self.keys, key)

	self.packing()
}

func (self *StrMap) Has(key string) bool {
	if self == nil {
		return false
	}
	_, ok := self.keys[key]
	return ok
}

func (self *StrMap) Iterate(cb func(key string, value interface{}) bool) {

	if self == nil {
		return
	}

	self.iterCounter++
	defer func() {
		self.iterCounter--
	}()

	for i := 0; i < len(self.values); i++ {

		if self.values[i].value == gDeletedEntry {
			continue
		}

		if !cb(self.values[i].key, self.values[i].value) {
			break
		}
	}
	self.packing()
}

func (self *StrMap) packing() {
	if self == nil || self.iterCounter > 0 {
		return
	}

	// Simple stupid algorithm
	if len(self.values) >= 128 && len(self.keys)*3 < len(self.values) {

		values := make([]mapEntry, len(self.keys))
		var j uint32 = 0
		for i := 0; i < len(self.values); i++ {

			if self.values[i].value == gDeletedEntry {
				continue
			}

			values[j] = self.values[i]
			self.keys[values[j].key] = j
			j++
		}
		self.values = values
	}
}
