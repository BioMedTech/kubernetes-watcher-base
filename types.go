package watcher


const (
	ActionDelete = "delete"
	ActionUpdate = "update"
	ActionCreate = "create"
)

type Change struct {
	Action string
	Key    string
}

func NewChange(action string, key string) *Change {
	return &Change{Action: action, Key: key}
}

